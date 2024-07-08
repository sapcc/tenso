/*******************************************************************************
*
* Copyright 2023 SAP SE
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
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/majewsky/schwift/v2"

	"github.com/sapcc/go-api-declarations/deployevent"

	"github.com/sapcc/tenso/internal/servicenow"
	"github.com/sapcc/tenso/internal/tenso"
)

func init() {
	tenso.ValidationHandlerRegistry.Add(func() tenso.ValidationHandler { return &terraformDeploymentValidator{} })
	tenso.DeliveryHandlerRegistry.Add(func() tenso.DeliveryHandler { return &terraformDeploymentToSwiftDeliverer{} })
	tenso.TranslationHandlerRegistry.Add(func() tenso.TranslationHandler { return &terraformDeploymentToSNowTranslator{} })
	tenso.DeliveryHandlerRegistry.Add(func() tenso.DeliveryHandler { return &terraformDeploymentToSNowDeliverer{} })
}

////////////////////////////////////////////////////////////////////////////////
// ValidationHandler

type terraformDeploymentValidator struct {
}

func (v *terraformDeploymentValidator) Init(context.Context, *gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}

func (v *terraformDeploymentValidator) PluginTypeID() string {
	return "terraform-deployment-from-concourse.v1"
}

func (v *terraformDeploymentValidator) ValidatePayload(payload []byte) (*tenso.PayloadInfo, error) {
	event, err := parseAndValidateDeployEvent(payload)
	if err != nil {
		return nil, err
	}

	if event.ADDeployment != nil {
		return nil, errors.New("active-directory-deployment may not be set for Terraform run events")
	}
	if len(event.HelmReleases) != 0 {
		return nil, errors.New("helm-release[] may not be set for Terraform run events")
	}
	if len(event.TerraformRuns) == 0 {
		return nil, errors.New("terraform-runs[] may not be empty")
	}

	for idx, runInfo := range event.TerraformRuns {
		if runInfo == nil {
			return nil, fmt.Errorf(`terraform-runs[%d] may not be nil`, idx)
		}
		if !runInfo.Outcome.IsKnownInputValue() {
			return nil, fmt.Errorf(`in terraform-runs[%d]: invalid value for field outcome: %q`, idx, runInfo.Outcome)
		}

		if runInfo.StartedAt == nil && runInfo.Outcome != deployevent.OutcomeNotDeployed {
			return nil, fmt.Errorf(`in terraform-runs[%d]: field started-at must be set for outcome %q`, idx, runInfo.Outcome)
		}
		if runInfo.StartedAt != nil && runInfo.Outcome == deployevent.OutcomeNotDeployed {
			return nil, fmt.Errorf(`in terraform-runs[%d]: field started-at may not be set for outcome %q`, idx, runInfo.Outcome)
		}
		if runInfo.FinishedAt == nil && (runInfo.Outcome != deployevent.OutcomeNotDeployed && runInfo.Outcome != deployevent.OutcomeHelmUpgradeFailed) {
			return nil, fmt.Errorf(`in terraform-runs[%d]: field finished-at must be set for outcome %q`, idx, runInfo.Outcome)
		}
		if runInfo.FinishedAt != nil && (runInfo.Outcome == deployevent.OutcomeNotDeployed || runInfo.Outcome == deployevent.OutcomeHelmUpgradeFailed) {
			return nil, fmt.Errorf(`in terraform-runs[%d]: field finished-at may not be set for outcome %q`, idx, runInfo.Outcome)
		}

		if runInfo.TerraformVersion == "" {
			return nil, fmt.Errorf(`in terraform-runs[%d]: field terraform-version may not be empty`, idx)
		}

		// Terraform will only show the change_summary if the operation completes successfully
		//Ref: <https://github.com/hashicorp/terraform/blob/6fa5784129f706a4b459b4495394899c6cc3e041/internal/command/apply.go#L131-L138>
		if runInfo.Outcome == deployevent.OutcomeSucceeded && runInfo.ChangeSummary == nil {
			return nil, fmt.Errorf(`in terraform-runs[%d]: field change-summary must be set for outcome %q`, idx, runInfo.Outcome)
		}
		if runInfo.Outcome != deployevent.OutcomeSucceeded && runInfo.ChangeSummary != nil {
			return nil, fmt.Errorf(`in terraform-runs[%d]: field change-summary may not be set for outcome %q`, idx, runInfo.Outcome)
		}

		if runInfo.Outcome == deployevent.OutcomeTerraformRunFailed && runInfo.ErrorMessage == "" {
			return nil, fmt.Errorf(`in terraform-runs[%d]: field terraform-version must be set for outcome %q`, idx, runInfo.Outcome)
		}
		if runInfo.Outcome != deployevent.OutcomeTerraformRunFailed && runInfo.ErrorMessage != "" {
			return nil, fmt.Errorf(`in terraform-runs[%d]: field terraform-version may not be set for outcome %q`, idx, runInfo.Outcome)
		}
	}

	return &tenso.PayloadInfo{
		Description: fmt.Sprintf("%s/%s: Terraform run for %s",
			event.Pipeline.TeamName, event.Pipeline.PipelineName, event.Pipeline.JobName),
	}, nil
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for Swift

type terraformDeploymentToSwiftDeliverer struct {
	Container *schwift.Container
}

func (h *terraformDeploymentToSwiftDeliverer) Init(ctx context.Context, pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) (err error) {
	h.Container, err = tenso.InitializeSwiftDelivery(ctx, pc, eo, "TENSO_TERRAFORM_DEPLOYMENT_SWIFT_CONTAINER")
	return err
}

func (h *terraformDeploymentToSwiftDeliverer) PluginTypeID() string {
	return "terraform-deployment-to-swift.v1"
}

func (h *terraformDeploymentToSwiftDeliverer) DeliverPayload(ctx context.Context, payload []byte, routingInfo map[string]string) (*tenso.DeliveryLog, error) {
	event, err := jsonUnmarshalStrict[deployevent.Event](payload)
	if err != nil {
		return nil, err
	}

	objectName := fmt.Sprintf("%s/%s/%s/%s/%s.json",
		event.Pipeline.TeamName, event.Pipeline.PipelineName,
		event.Pipeline.JobName,
		string(event.CombinedOutcome()),
		event.RecordedAt.Format(time.RFC3339),
	)
	return nil, h.Container.Object(objectName).Upload(ctx, bytes.NewReader(payload), nil, nil)
}

////////////////////////////////////////////////////////////////////////////////
// TranslationHandler for SNow

type terraformDeploymentToSNowTranslator struct {
	Mapping servicenow.MappingConfiguration
}

func (t *terraformDeploymentToSNowTranslator) Init(context.Context, *gophercloud.ProviderClient, gophercloud.EndpointOpts) (err error) {
	t.Mapping, err = servicenow.LoadMappingConfiguration("TENSO_SERVICENOW_MAPPING_CONFIG_PATH")
	return err
}

func (t *terraformDeploymentToSNowTranslator) PluginTypeID() string {
	return "terraform-deployment-from-concourse.v1->terraform-deployment-to-servicenow.v1"
}

func (t *terraformDeploymentToSNowTranslator) TranslatePayload(payload []byte, routingInfo map[string]string) ([]byte, error) {
	event, err := jsonUnmarshalStrict[deployevent.Event](payload)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(event.TerraformRuns, func(i, j int) bool {
		lhs := event.TerraformRuns[i].StartedAt
		if lhs == nil {
			return true
		}
		rhs := event.TerraformRuns[j].StartedAt
		if rhs == nil {
			return false
		}
		return lhs.Before(*rhs)
	})

	inputDesc := strings.Join(inputDescriptorsOf(event), ", ")
	descLines := []string{
		fmt.Sprintf("Deployed %s for %s with versions: %s", event.Pipeline.PipelineName, event.Pipeline.JobName, inputDesc),
	}
	for idx, run := range event.TerraformRuns {
		descLines = append(descLines, fmt.Sprintf("Step %d: %s", idx+1, summaryOfRun(*run)))
	}
	descLines = append(descLines,
		"Deployment log: "+event.Pipeline.BuildURL,
		"",
		fmt.Sprintf("Outcome: %s", event.CombinedOutcome()),
	)

	chg := servicenow.Change{
		StartedAt:   event.CombinedStartDate(),
		EndedAt:     event.RecordedAt,
		Outcome:     event.CombinedOutcome(),
		Summary:     fmt.Sprintf("Deploy %s for %s", event.Pipeline.PipelineName, event.Pipeline.JobName),
		Description: strings.Join(descLines, "\n"),
		Executee:    event.Pipeline.CreatedBy, //NOTE: can be empty
		Region:      event.Region,
	}

	return chg.Serialize(t.Mapping, t.Mapping.TerraformDeployment)
}

func summaryOfRun(run deployevent.TerraformRun) string {
	var parts []string
	if run.ChangeSummary.Added > 0 {
		parts = append(parts, fmt.Sprintf("added %d objects", run.ChangeSummary.Added))
	}
	if run.ChangeSummary.Changed > 0 {
		parts = append(parts, fmt.Sprintf("changed %d objects", run.ChangeSummary.Changed))
	}
	if run.ChangeSummary.Removed > 0 {
		parts = append(parts, fmt.Sprintf("removed %d objects", run.ChangeSummary.Removed))
	}
	if len(parts) == 0 {
		parts = append(parts, "no changes")
	}
	return fmt.Sprintf("%s (outcome: %s)", strings.Join(parts, ", "), run.Outcome)
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for SNow

type terraformDeploymentToSNowDeliverer struct {
	Mapping servicenow.MappingConfiguration
}

func (d *terraformDeploymentToSNowDeliverer) Init(context.Context, *gophercloud.ProviderClient, gophercloud.EndpointOpts) (err error) {
	d.Mapping, err = servicenow.LoadMappingConfiguration("TENSO_SERVICENOW_MAPPING_CONFIG_PATH")
	return err
}

func (d *terraformDeploymentToSNowDeliverer) PluginTypeID() string {
	return "terraform-deployment-to-servicenow.v1"
}

func (d *terraformDeploymentToSNowDeliverer) DeliverPayload(ctx context.Context, payload []byte, routingInfo map[string]string) (*tenso.DeliveryLog, error) {
	return d.Mapping.Endpoints.DeliverChangePayload(ctx, payload, routingInfo)
}
