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
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/majewsky/schwift"
	"github.com/majewsky/schwift/gopherschwift"
	"github.com/sapcc/go-api-declarations/helmevent"
	"github.com/sapcc/go-bits/osext"

	"github.com/sapcc/tenso/internal/servicenow"
	"github.com/sapcc/tenso/internal/tenso"
)

func init() {
	tenso.ValidationHandlerRegistry.Add(func() tenso.ValidationHandler { return &helmDeploymentValidator{} })
	tenso.DeliveryHandlerRegistry.Add(func() tenso.DeliveryHandler { return &helmDeploymentToElkDeliverer{} })
	tenso.DeliveryHandlerRegistry.Add(func() tenso.DeliveryHandler { return &helmDeploymentToSwiftDeliverer{} })
	tenso.TranslationHandlerRegistry.Add(func() tenso.TranslationHandler { return &helmDeploymentToSNowTranslator{} })
	tenso.DeliveryHandlerRegistry.Add(func() tenso.DeliveryHandler { return &helmDeploymentToSNowDeliverer{} })
}

////////////////////////////////////////////////////////////////////////////////
// helper functions

func releaseDescriptorsOf(event helmevent.Event, sep string) (result []string) {
	for _, hr := range event.HelmReleases {
		result = append(result, fmt.Sprintf("%s%s%s", hr.Name, sep, hr.Cluster))
	}
	return
}

func inputDescriptorsOf(event helmevent.Event) (result []string) {
	var imageVersions []string
	for _, rel := range event.HelmReleases {
		if rel.ImageVersion != "" {
			imageVersions = append(imageVersions, fmt.Sprintf("%s %s", rel.Name, rel.ImageVersion))
		}
	}

	var gitVersions []string
	for name, repo := range event.GitRepos {
		gitVersions = append(gitVersions, fmt.Sprintf("%s.git %s", name, repo.CommitID))
	}
	sort.Strings(gitVersions) //for test reproducability

	return append(imageVersions, gitVersions...)
}

////////////////////////////////////////////////////////////////////////////////
// ValidationHandler

type helmDeploymentValidator struct {
}

func (h *helmDeploymentValidator) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}

func (h *helmDeploymentValidator) PluginTypeID() string {
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
	event, err := jsonUnmarshalStrict[helmevent.Event](payload)
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
		if relInfo.StartedAt == nil && relInfo.Outcome != helmevent.OutcomeNotDeployed {
			return nil, fmt.Errorf(`in helm-release %q: field started-at must be set for outcome %q`, relInfo.Name, relInfo.Outcome)
		}
		if relInfo.StartedAt != nil && relInfo.Outcome == helmevent.OutcomeNotDeployed {
			return nil, fmt.Errorf(`in helm-release %q: field started-at may not be set for outcome %q`, relInfo.Name, relInfo.Outcome)
		}
		if relInfo.FinishedAt == nil && (relInfo.Outcome != helmevent.OutcomeNotDeployed && relInfo.Outcome != helmevent.OutcomeHelmUpgradeFailed) {
			return nil, fmt.Errorf(`in helm-release %q: field finished-at must be set for outcome %q`, relInfo.Name, relInfo.Outcome)
		}
		if relInfo.FinishedAt != nil && (relInfo.Outcome == helmevent.OutcomeNotDeployed || relInfo.Outcome == helmevent.OutcomeHelmUpgradeFailed) {
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
			strings.Join(releaseDescriptorsOf(event, " to "), " and "),
		),
	}, nil
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for ELK

// helmDeploymentToElkDeliverer is a tenso.DeliveryHandler.
type helmDeploymentToElkDeliverer struct {
	LogstashHost string
}

func (h *helmDeploymentToElkDeliverer) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	var err error
	h.LogstashHost, err = osext.NeedGetenv("TENSO_HELM_DEPLOYMENT_LOGSTASH_HOST")
	if err != nil {
		return err
	}
	_, _, err = net.SplitHostPort(h.LogstashHost)
	if err != nil {
		return fmt.Errorf(`expected TENSO_HELM_DEPLOYMENT_LOGSTASH_HOST to look like "host:port", but got %q`,
			h.LogstashHost)
	}
	return nil
}

func (h *helmDeploymentToElkDeliverer) PluginTypeID() string {
	return "helm-deployment-to-elk.v1"
}

func (h *helmDeploymentToElkDeliverer) DeliverPayload(payload []byte) (*tenso.DeliveryLog, error) {
	//Logstash wants everything on one line, so ensure we don't have unnecessary whitespace in the payload
	var buf bytes.Buffer
	err := json.Compact(&buf, payload)
	if err != nil {
		return nil, err
	}
	err = buf.WriteByte('\n')
	if err != nil {
		return nil, err
	}
	payload = buf.Bytes()

	//deliver payload to Logstash
	conn, err := net.Dial("tcp", h.LogstashHost)
	if err != nil {
		return nil, err
	}
	_, err = conn.Write(payload)
	if err != nil {
		return nil, err
	}
	return nil, conn.Close()
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for Swift

type helmDeploymentToSwiftDeliverer struct {
	Container *schwift.Container
}

func (h *helmDeploymentToSwiftDeliverer) Init(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) error {
	containerName, err := osext.NeedGetenv("TENSO_HELM_DEPLOYMENT_SWIFT_CONTAINER")
	if err != nil {
		return err
	}

	client, err := openstack.NewObjectStorageV1(pc, eo)
	if err != nil {
		return err
	}

	swiftAccount, err := gopherschwift.Wrap(client, nil)
	if err != nil {
		return err
	}

	h.Container, err = swiftAccount.Container(containerName).EnsureExists()
	return err
}

func (h *helmDeploymentToSwiftDeliverer) PluginTypeID() string {
	return "helm-deployment-to-swift.v1"
}

func (h *helmDeploymentToSwiftDeliverer) DeliverPayload(payload []byte) (*tenso.DeliveryLog, error) {
	event, err := jsonUnmarshalStrict[helmevent.Event](payload)
	if err != nil {
		return nil, err
	}

	objectName := fmt.Sprintf("%s/%s/%s/%s/%s.json",
		event.Pipeline.TeamName, event.Pipeline.PipelineName,
		strings.Join(releaseDescriptorsOf(event, "_"), ","),
		string(event.CombinedOutcome()),
		event.RecordedAt.Format(time.RFC3339),
	)
	return nil, h.Container.Object(objectName).Upload(bytes.NewReader(payload), nil, nil)
}

////////////////////////////////////////////////////////////////////////////////
// TranslationHandler for SNow

type helmDeploymentToSNowTranslator struct {
	Mapping servicenow.MappingConfiguration
}

var helmSNowCloseCodes = map[helmevent.Outcome]string{
	helmevent.OutcomeNotDeployed: "Failed - Rolled back",
	//This used to be "Partially Implemented" and "Failed - Others", but it was
	//all changed to "Closed without Implementation" because the former close
	//codes are intended for problems that require human intervention and
	//subsequent analysis, which we do not want.
	helmevent.OutcomePartiallyDeployed: "Closed without Implementation",
	helmevent.OutcomeHelmUpgradeFailed: "Closed without Implementation",
	helmevent.OutcomeE2ETestFailed:     "Closed without Implementation",
	helmevent.OutcomeSucceeded:         "Implemented - Successfully",
}

func (h *helmDeploymentToSNowTranslator) Init(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) (err error) {
	h.Mapping, err = servicenow.LoadMappingConfiguration()
	return err
}

func (h *helmDeploymentToSNowTranslator) PluginTypeID() string {
	return "helm-deployment-from-concourse.v1->helm-deployment-to-servicenow.v1"
}

func (h *helmDeploymentToSNowTranslator) TranslatePayload(payload []byte) ([]byte, error) {
	event, err := jsonUnmarshalStrict[helmevent.Event](payload)
	if err != nil {
		return nil, err
	}

	//if we did not start deploying, we won't create a change object in ServiceNow
	outcome := event.CombinedOutcome()
	if outcome == helmevent.OutcomeNotDeployed {
		return []byte("skip"), nil
	}

	releaseDesc := strings.Join(releaseDescriptorsOf(event, " to "), ", ")
	inputDesc := strings.Join(inputDescriptorsOf(event), ", ")
	chg := servicenow.Change{
		StartedAt:   event.CombinedStartDate(),
		EndedAt:     event.RecordedAt,
		CloseCode:   helmSNowCloseCodes[event.CombinedOutcome()],
		Summary:     fmt.Sprintf("Deploy %s", releaseDesc),
		Description: fmt.Sprintf("Deployed %s with versions: %s\nDeployment log: %s\n\nOutcome: %s", releaseDesc, inputDesc, event.Pipeline.BuildURL, string(event.CombinedOutcome())),
		Executee:    event.Pipeline.CreatedBy, //NOTE: can be empty
		Region:      event.Region,
	}

	return chg.Serialize(h.Mapping, h.Mapping.HelmDeployment)
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for SNow

type helmDeploymentToSNowDeliverer struct {
	Client *servicenow.Client
}

func (h *helmDeploymentToSNowDeliverer) Init(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) (err error) {
	h.Client, err = servicenow.NewClientWithOAuth("TENSO_SERVICENOW")
	return err
}

func (h *helmDeploymentToSNowDeliverer) PluginTypeID() string {
	return "helm-deployment-to-servicenow.v1"
}

func (h *helmDeploymentToSNowDeliverer) DeliverPayload(payload []byte) (*tenso.DeliveryLog, error) {
	//if the TranslationHandler wants us to ignore this payload, skip the delivery
	if string(payload) == "skip" {
		return nil, nil
	}
	return h.Client.DeliverChangePayload(payload)
}
