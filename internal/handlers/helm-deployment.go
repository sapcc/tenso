/*******************************************************************************
*
* Copyright 2022 SAP SE
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You should have received a copy of the License along with this
* program. If not, you may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*
*******************************************************************************/

package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/majewsky/schwift"
	"github.com/majewsky/schwift/gopherschwift"

	"github.com/sapcc/tenso/internal/servicenow"
	"github.com/sapcc/tenso/internal/tenso"
)

func init() {
	tenso.RegisterValidationHandler(&helmDeploymentValidator{})
	tenso.RegisterDeliveryHandler(&helmDeploymentToElkDeliverer{})
	tenso.RegisterDeliveryHandler(&helmDeploymentToSwiftDeliverer{})
	tenso.RegisterTranslationHandler(&helmDeploymentToSNowTranslator{})
	tenso.RegisterDeliveryHandler(&helmDeploymentToSNowDeliverer{})
}

////////////////////////////////////////////////////////////////////////////////
// data types ("Hd..." = Helm deployment)

type HdEvent struct {
	Region       string               `json:"region"`
	RecordedAt   *time.Time           `json:"recorded_at"`
	GitRepos     map[string]HdGitRepo `json:"git"`
	HelmReleases []*HdHelmRelease     `json:"helm-release"`
	Pipeline     HdPipeline           `json:"pipeline"`
}

type HdGitRepo struct {
	CommitID string `json:"commit-id"`
}

type HdHelmRelease struct {
	Name    string    `json:"name"`
	Outcome HdOutcome `json:"outcome"`

	//ChartID contains "${name}-${version}" for charts pulled from Chartmuseum.
	//ChartPath contains the path to that chart inside helm-charts.git for charts
	//coming from helm-charts.git directly. Exactly one of those must be set.
	ChartID   string `json:"chart-id"`
	ChartPath string `json:"chart-path"`
	Cluster   string `json:"cluster"`
	//ImageVersion is only set for releases that take an image version produced by an earlier pipeline job.
	ImageVersion string `json:"image-version,omitempty"`
	Namespace    string `json:"kubernetes-namespace"`

	//StartedAt is not set for HdOutcomeNotDeployed.
	StartedAt *time.Time `json:"started-at"`
	//FinishedAt is not set for HdOutcomeNotDeployed and HdOutcomeHelmUpgradeFailed.
	FinishedAt      *time.Time `json:"finished-at,omitempty"`
	DurationSeconds *uint64    `json:"duration,omitempty"`
}

//HdOutcome describes the final state of a Helm release.
type HdOutcome string

const (
	//HdOutcomeNotDeployed describes a Helm release that was not deployed because
	//of an unexpected error before `helm upgrade`.
	HdOutcomeNotDeployed HdOutcome = "not-deployed"
	//HdOutcomeSucceeded describes a Helm release that succeeded.
	HdOutcomeSucceeded HdOutcome = "succeeded"
	//HdOutcomeHelmUpgradeFailed describes a Helm release that failed during
	//`helm upgrade` or because some deployed pods did not come up correctly.
	HdOutcomeHelmUpgradeFailed HdOutcome = "helm-upgrade-failed"
	//HdOutcomeE2ETestFailed describes a Helm release that was deployed, but a
	//subsequent end-to-end test failed.
	HdOutcomeE2ETestFailed HdOutcome = "e2e-test-failed"
	//HdOutcomePartiallyDeployed is returned by CombinedOutcome() when the event
	//in question contains some releases that are "succeeded" and some that are
	//"not-deployed". This value is not acceptable for an individual Helm release.
	HdOutcomePartiallyDeployed HdOutcome = "partially-deployed"
)

func (o HdOutcome) IsKnownInputValue() bool {
	switch o {
	case HdOutcomeNotDeployed, HdOutcomeSucceeded, HdOutcomeHelmUpgradeFailed, HdOutcomeE2ETestFailed:
		return true
	case HdOutcomePartiallyDeployed:
		return false //not acceptable on an individual release, can only appear as result of HdEvent.CombinedOutcome()
	default:
		return false
	}
}

type HdPipeline struct {
	BuildNumber  string `json:"build-number"`
	BuildURL     string `json:"build-url"`
	JobName      string `json:"job"`
	PipelineName string `json:"name"`
	TeamName     string `json:"team"`
	CreatedBy    string `json:"created-by"`
}

////////////////////////////////////////////////////////////////////////////////
// helper functions on HdEvent

func (event HdEvent) ReleaseDescriptors(sep string) (result []string) {
	for _, hr := range event.HelmReleases {
		result = append(result, fmt.Sprintf("%s%s%s", hr.Name, sep, hr.Cluster))
	}
	return
}

func (event HdEvent) CombinedOutcome() HdOutcome {
	hasSucceeded := false
	hasUndeployed := false
	for _, hr := range event.HelmReleases {
		switch hr.Outcome {
		case HdOutcomeHelmUpgradeFailed, HdOutcomeE2ETestFailed:
			//specific failure forces the entire result to be that failure
			return hr.Outcome
		case HdOutcomeSucceeded:
			hasSucceeded = true
		case HdOutcomeNotDeployed:
			hasUndeployed = true
		}
	}

	switch {
	case hasSucceeded && hasUndeployed:
		return HdOutcomePartiallyDeployed
	case hasSucceeded:
		return HdOutcomeSucceeded
	default:
		return HdOutcomeNotDeployed
	}
}

////////////////////////////////////////////////////////////////////////////////
// ValidationHandler

type helmDeploymentValidator struct {
}

func (h *helmDeploymentValidator) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}

func (h *helmDeploymentValidator) PayloadType() string {
	return "helm-deployment-from-concourse.v1"
}

var (
	regionRx      = regexp.MustCompile(`^[a-z]{2}-[a-z]{2}-[0-9]$`)                       //e.g. "qa-de-1"
	clusterRx     = regexp.MustCompile(`^(?:|[a-z]-|ci[0-9]?-)?[a-z]{2}-[a-z]{2}-[0-9]$`) //e.g. "qa-de-1" or "s-qa-de-1" or "ci-eu-de-2"
	gitCommitRx   = regexp.MustCompile(`^[0-9a-f]{40}$`)                                  //SHA-1 digest with lower-case digits
	buildNumberRx = regexp.MustCompile(`^[1-9][0-9]*(?:\.[1-9][0-9]*)?$`)                 //e.g. "23" or "42.1"
	sapUserIDRx   = regexp.MustCompile(`^(?:C[0-9]{7}|[DI][0-9]{6})$`)                    //e.g. "D123456" or "C1234567"
)

func (h *helmDeploymentValidator) ValidatePayload(payload []byte) (*tenso.PayloadInfo, error) {
	var event HdEvent
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	err := dec.Decode(&event)
	if err != nil {
		return nil, err
	}

	if !regionRx.MatchString(event.Region) {
		return nil, fmt.Errorf(`value for field region is invalid: %q`, event.Region)
	}
	if event.RecordedAt == nil {
		return nil, errors.New("value for field recorded_at is missing")
	}

	for repoName, repoInfo := range event.GitRepos {
		if !gitCommitRx.MatchString(repoInfo.CommitID) {
			return nil, fmt.Errorf(`value for field git[%q].commit-id is invalid: %q`, repoName, repoInfo.CommitID)
		}
	}

	if len(event.HelmReleases) == 0 {
		return nil, errors.New("helm-release[] may not be empty")
	}
	for _, relInfo := range event.HelmReleases {
		//TODO: Can we do regex matches to validate the contents of Name, Namespace, ChartID, ChartPath?
		if relInfo.Name == "" {
			return nil, fmt.Errorf(`invalid value for field helm-release[].name: %q`, relInfo.Name)
		}
		if !relInfo.Outcome.IsKnownInputValue() {
			return nil, fmt.Errorf(`in helm-release %q: invalid value for field outcome: %q`, relInfo.Name, relInfo.Outcome)
		}
		if relInfo.ChartID == "" && relInfo.ChartPath == "" {
			return nil, fmt.Errorf(`in helm-release %q: chart-id and chart-path can not both be empty`, relInfo.Name)
		}
		if relInfo.ChartID != "" && relInfo.ChartPath != "" {
			return nil, fmt.Errorf(`in helm-release %q: chart-id and chart-path can not both be set`, relInfo.Name)
		}
		if !clusterRx.MatchString(relInfo.Cluster) {
			return nil, fmt.Errorf(`in helm-release %q: invalid value for field cluster: %q`, relInfo.Name, relInfo.Cluster)
		}
		if !strings.HasSuffix(relInfo.Cluster, event.Region) {
			return nil, fmt.Errorf(`in helm-release %q: cluster %q is not in region %q`, relInfo.Name, relInfo.Cluster, event.Region)
		}
		if relInfo.Namespace == "" {
			return nil, fmt.Errorf(`in helm-release %q: invalid value for field namespace: %q`, relInfo.Name, relInfo.Namespace)
		}
		if relInfo.StartedAt == nil && relInfo.Outcome != HdOutcomeNotDeployed {
			return nil, fmt.Errorf(`in helm-release %q: field started-at must be set for outcome %q`, relInfo.Name, relInfo.Outcome)
		}
		if relInfo.StartedAt != nil && relInfo.Outcome == HdOutcomeNotDeployed {
			return nil, fmt.Errorf(`in helm-release %q: field started-at may not be set for outcome %q`, relInfo.Name, relInfo.Outcome)
		}
		if relInfo.FinishedAt == nil && (relInfo.Outcome != HdOutcomeNotDeployed && relInfo.Outcome != HdOutcomeHelmUpgradeFailed) {
			return nil, fmt.Errorf(`in helm-release %q: field finished-at must be set for outcome %q`, relInfo.Name, relInfo.Outcome)
		}
		if relInfo.FinishedAt != nil && (relInfo.Outcome == HdOutcomeNotDeployed || relInfo.Outcome == HdOutcomeHelmUpgradeFailed) {
			return nil, fmt.Errorf(`in helm-release %q: field finished-at may not be set for outcome %q`, relInfo.Name, relInfo.Outcome)
		}
	}

	//TODO: Can we validate values for TeamName by providing a set of valid values in env?
	if !buildNumberRx.MatchString(event.Pipeline.BuildNumber) {
		return nil, fmt.Errorf("field pipeline.build-number is invalid: %q", event.Pipeline.BuildNumber)
	}
	_, err = url.Parse(event.Pipeline.BuildURL)
	if err != nil {
		return nil, fmt.Errorf("field pipeline.build-url is invalid: %q", event.Pipeline.BuildURL)
	}
	if event.Pipeline.JobName == "" {
		return nil, fmt.Errorf("field pipeline.job is invalid: %q", event.Pipeline.JobName)
	}
	if event.Pipeline.PipelineName == "" {
		return nil, fmt.Errorf("field pipeline.name is invalid: %q", event.Pipeline.PipelineName)
	}
	if event.Pipeline.TeamName == "" {
		return nil, fmt.Errorf("field pipeline.team is invalid: %q", event.Pipeline.TeamName)
	}
	if event.Pipeline.CreatedBy != "" && !sapUserIDRx.MatchString(event.Pipeline.CreatedBy) {
		return nil, fmt.Errorf("field pipeline.created-by is invalid: %q", event.Pipeline.CreatedBy)
	}

	return &tenso.PayloadInfo{
		Description: fmt.Sprintf("%s/%s: deploy %s",
			event.Pipeline.TeamName, event.Pipeline.PipelineName,
			strings.Join(event.ReleaseDescriptors(" to "), " and "),
		),
	}, nil
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for ELK

//helmDeploymentToElkDeliverer is a tenso.DeliveryHandler.
type helmDeploymentToElkDeliverer struct {
	LogstashHost string
}

func (h *helmDeploymentToElkDeliverer) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	h.LogstashHost = os.Getenv("TENSO_HELM_DEPLOYMENT_LOGSTASH_HOST")
	if h.LogstashHost == "" {
		return errors.New("missing required environment variable: TENSO_HELM_DEPLOYMENT_LOGSTASH_HOST")
	}
	_, _, err := net.SplitHostPort(h.LogstashHost)
	if err != nil {
		return fmt.Errorf(`expected TENSO_HELM_DEPLOYMENT_LOGSTASH_HOST to look like "host:port", but got %q`,
			h.LogstashHost)
	}
	return nil
}

func (h *helmDeploymentToElkDeliverer) PayloadType() string {
	return "helm-deployment-to-elk.v1"
}

func (h *helmDeploymentToElkDeliverer) DeliverPayload(payload []byte) error {
	//Logstash wants everything on one line, so ensure we don't have unnecessary whitespace in the payload
	var buf bytes.Buffer
	err := json.Compact(&buf, payload)
	if err != nil {
		return err
	}
	err = buf.WriteByte('\n')
	if err != nil {
		return err
	}
	payload = buf.Bytes()

	//deliver payload to Logstash
	conn, err := net.Dial("tcp", h.LogstashHost)
	if err != nil {
		return err
	}
	_, err = conn.Write(payload)
	if err != nil {
		return err
	}
	return conn.Close()
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for Swift

type helmDeploymentToSwiftDeliverer struct {
	Container *schwift.Container
}

func (h *helmDeploymentToSwiftDeliverer) Init(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) error {
	containerName := os.Getenv("TENSO_HELM_DEPLOYMENT_SWIFT_CONTAINER")
	if containerName == "" {
		return errors.New("missing required environment variable: TENSO_HELM_DEPLOYMENT_SWIFT_CONTAINER")
	}

	client, err := openstack.NewObjectStorageV1(pc, eo)
	if err != nil {
		return err
	}

	swiftAccount, err := gopherschwift.Wrap(client, &gopherschwift.Options{
		UserAgent: fmt.Sprintf("%s/rolling", tenso.Component),
	})
	if err != nil {
		return err
	}

	h.Container, err = swiftAccount.Container(containerName).EnsureExists()
	return err
}

func (h *helmDeploymentToSwiftDeliverer) PayloadType() string {
	return "helm-deployment-to-swift.v1"
}

func (h *helmDeploymentToSwiftDeliverer) DeliverPayload(payload []byte) error {
	var event HdEvent
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	err := dec.Decode(&event)
	if err != nil {
		return err
	}

	objectName := fmt.Sprintf("%s/%s/%s/%s/%s.json",
		event.Pipeline.TeamName, event.Pipeline.PipelineName,
		strings.Join(event.ReleaseDescriptors("_"), ","),
		string(event.CombinedOutcome()),
		event.RecordedAt.Format(time.RFC3339),
	)
	return h.Container.Object(objectName).Upload(bytes.NewReader(payload), nil, nil)
}

////////////////////////////////////////////////////////////////////////////////
// TranslationHandler for SNow

type helmDeploymentToSNowTranslator struct{}

func (h *helmDeploymentToSNowTranslator) Init(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) error {
	return nil
}

func (h *helmDeploymentToSNowTranslator) SourcePayloadType() string {
	return "helm-deployment-from-concourse.v1"
}

func (h *helmDeploymentToSNowTranslator) TargetPayloadType() string {
	return "helm-deployment-to-servicenow.v1"
}

func (h *helmDeploymentToSNowTranslator) TranslatePayload(payload []byte) ([]byte, error) {
	return nil, errors.New("TODO unimplemented")
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for SNow

type helmDeploymentToSNowDeliverer struct {
	EndpointURL string
	HTTPClient  *http.Client
}

func (h *helmDeploymentToSNowDeliverer) Init(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) (err error) {
	h.EndpointURL = os.Getenv("TENSO_SERVICENOW_CREATE_CHANGE_URL")
	if h.EndpointURL == "" {
		return errors.New("missing required environment variable: TENSO_SERVICENOW_CREATE_CHANGE_URL")
	}
	h.HTTPClient, err = servicenow.NewClientWithOAuth("TENSO_SERVICENOW")
	return err
}

func (h *helmDeploymentToSNowDeliverer) PayloadType() string {
	return "helm-deployment-to-servicenow.v1"
}

func (h *helmDeploymentToSNowDeliverer) DeliverPayload(payload []byte) error {
	req, err := http.NewRequest("POST", h.EndpointURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("while preparing request for POST %s: %w", h.EndpointURL, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("during POST %s: %w", h.EndpointURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 400 {
		return nil
	}

	//unexpected error -> log response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("while reading response body for failed POST %s: %w", h.EndpointURL, err)
	}
	return fmt.Errorf("POST failed with status %d and response: %q", resp.StatusCode, string(bodyBytes))
}
