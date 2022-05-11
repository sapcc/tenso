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
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/sapcc/tenso/internal/tenso"
)

func init() {
	tenso.RegisterValidationHandler(&helmDeploymentValidator{})
	tenso.RegisterDeliveryHandler(&helmDeploymentToElkDeliverer{})
}

////////////////////////////////////////////////////////////////////////////////
// data types ("Hd..." = Helm deployment)

type HdEvent struct {
	Region       string               `json:"region"`
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
	//HdOutcomeSucceeded describes a Helm release that succeded.
	HdOutcomeSucceeded HdOutcome = "succeeded"
	//HdOutcomeHelmUpgradeFailed describes a Helm release that failed during
	//`helm upgrade` or because some deployed pods did not come up correctly.
	HdOutcomeHelmUpgradeFailed HdOutcome = "helm-upgrade-failed"
	//HdOutcomeE2ETestFailed describes a Helm release that was deployed, but a
	//subsequent end-to-end test failed.
	HdOutcomeE2ETestFailed HdOutcome = "e2e-test-failed"
)

func (o HdOutcome) IsKnownValue() bool {
	switch o {
	case HdOutcomeNotDeployed, HdOutcomeSucceeded, HdOutcomeHelmUpgradeFailed, HdOutcomeE2ETestFailed:
		return true
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
// ValidationHandler

type helmDeploymentValidator struct {
}

func (h *helmDeploymentValidator) Init() error {
	return nil
}

func (h *helmDeploymentValidator) PayloadType() string {
	return "helm-deployment-from-concourse.v1"
}

var (
	regionRx      = regexp.MustCompile(`^[a-z]{2}-[a-z]{2}-[0-9]$`)                      //e.g. "qa-de-1"
	clusterRx     = regexp.MustCompile(`^(?:|[a-z]-|ci[0-9]-)?[a-z]{2}-[a-z]{2}-[0-9]$`) //e.g. "qa-de-1" or "s-qa-de-1" or "ci1-eu-de-2"
	gitCommitRx   = regexp.MustCompile(`^[0-9a-f]{40}$`)                                 //SHA-1 digest with lower-case digits
	buildNumberRx = regexp.MustCompile(`^[1-9][0-9]*(?:\.[1-9][0-9]*)?$`)                //e.g. "23" or "42.1"
	sapUserIDRx   = regexp.MustCompile(`^(?:C[0-9]{7}|[DI][0-9]{6})$`)                   //e.g. "D123456" or "C1234567"
)

func (h *helmDeploymentValidator) ValidatePayload(payload []byte) error {
	var event HdEvent
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	err := dec.Decode(&event)
	if err != nil {
		return err
	}

	if !regionRx.MatchString(event.Region) {
		return fmt.Errorf(`value for field region is invalid: %q`, event.Region)
	}

	for repoName, repoInfo := range event.GitRepos {
		if !gitCommitRx.MatchString(repoInfo.CommitID) {
			return fmt.Errorf(`value for field git[%q].commit-id is invalid: %q`, repoName, repoInfo.CommitID)
		}
	}

	for _, relInfo := range event.HelmReleases {
		//TODO: Can we do regex matches to validate the contents of Name, Namespace, ChartID, ChartPath?
		if relInfo.Name == "" {
			return fmt.Errorf(`invalid value for field helm-release[].name: %q`, relInfo.Name)
		}
		if !relInfo.Outcome.IsKnownValue() {
			return fmt.Errorf(`in helm-release %q: invalid value for field outcome: %q`, relInfo.Name, relInfo.Outcome)
		}
		if relInfo.ChartID == "" && relInfo.ChartPath == "" {
			return fmt.Errorf(`in helm-release %q: chart-id and chart-path can not both be empty`, relInfo.Name)
		}
		if relInfo.ChartID != "" && relInfo.ChartPath != "" {
			return fmt.Errorf(`in helm-release %q: chart-id and chart-path can not both be set`, relInfo.Name)
		}
		if !clusterRx.MatchString(relInfo.Cluster) {
			return fmt.Errorf(`in helm-release %q: invalid value for field cluster: %q`, relInfo.Name, relInfo.Cluster)
		}
		if !strings.HasSuffix(relInfo.Cluster, event.Region) {
			return fmt.Errorf(`in helm-release %q: cluster %q is not in region %q`, relInfo.Name, relInfo.Cluster, event.Region)
		}
		if relInfo.Namespace == "" {
			return fmt.Errorf(`in helm-release %q: invalid value for field namespace: %q`, relInfo.Name, relInfo.Namespace)
		}
		if relInfo.StartedAt == nil && relInfo.Outcome != HdOutcomeNotDeployed {
			return fmt.Errorf(`in helm-release %q: field started-at must be set for outcome %q`, relInfo.Name, relInfo.Outcome)
		}
		if relInfo.StartedAt != nil && relInfo.Outcome == HdOutcomeNotDeployed {
			return fmt.Errorf(`in helm-release %q: field started-at may not be set for outcome %q`, relInfo.Name, relInfo.Outcome)
		}
		if relInfo.FinishedAt == nil && (relInfo.Outcome != HdOutcomeNotDeployed && relInfo.Outcome != HdOutcomeHelmUpgradeFailed) {
			return fmt.Errorf(`in helm-release %q: field finished-at must be set for outcome %q`, relInfo.Name, relInfo.Outcome)
		}
		if relInfo.FinishedAt != nil && (relInfo.Outcome == HdOutcomeNotDeployed || relInfo.Outcome == HdOutcomeHelmUpgradeFailed) {
			return fmt.Errorf(`in helm-release %q: field finished-at may not be set for outcome %q`, relInfo.Name, relInfo.Outcome)
		}
	}

	//TODO: Can we validate values for TeamName by providing a set of valid values in env?
	if !buildNumberRx.MatchString(event.Pipeline.BuildNumber) {
		return fmt.Errorf("field pipeline.build-number is invalid: %q", event.Pipeline.BuildNumber)
	}
	_, err = url.Parse(event.Pipeline.BuildURL)
	if err != nil {
		return fmt.Errorf("field pipeline.build-url is invalid: %q", event.Pipeline.BuildURL)
	}
	if event.Pipeline.JobName == "" {
		return fmt.Errorf("field pipeline.job is invalid: %q", event.Pipeline.JobName)
	}
	if event.Pipeline.PipelineName == "" {
		return fmt.Errorf("field pipeline.name is invalid: %q", event.Pipeline.PipelineName)
	}
	if event.Pipeline.TeamName == "" {
		return fmt.Errorf("field pipeline.team is invalid: %q", event.Pipeline.TeamName)
	}
	if event.Pipeline.CreatedBy != "" && !sapUserIDRx.MatchString(event.Pipeline.CreatedBy) {
		return fmt.Errorf("field pipeline.created-by is invalid: %q", event.Pipeline.CreatedBy)
	}

	return nil
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for ELK

//helmDeploymentToElkDeliverer is a tenso.DeliveryHandler.
type helmDeploymentToElkDeliverer struct{}

func (h *helmDeploymentToElkDeliverer) Init() error {
	return nil
}

func (h *helmDeploymentToElkDeliverer) PayloadType() string {
	return "helm-deployment-to-elk.v1"
}

func (h *helmDeploymentToElkDeliverer) DeliverPayload(payload []byte) error {
	return errors.New("TODO: implement delivery")
}
