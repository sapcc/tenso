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
	"errors"
	"fmt"

	"github.com/gophercloud/gophercloud"

	"github.com/sapcc/go-api-declarations/deployevent"

	"github.com/sapcc/tenso/internal/tenso"
)

func init() {
	tenso.ValidationHandlerRegistry.Add(func() tenso.ValidationHandler { return &terraformDeploymentValidator{} })
	tenso.TranslationHandlerRegistry.Add(func() tenso.TranslationHandler { return &terraformDeploymentToSNowTranslator{} })
	tenso.DeliveryHandlerRegistry.Add(func() tenso.DeliveryHandler { return &terraformDeploymentToSNowDeliverer{} })
}

////////////////////////////////////////////////////////////////////////////////
// ValidationHandler

type terraformDeploymentValidator struct {
}

func (v *terraformDeploymentValidator) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
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

	if len(event.HelmReleases) != 0 {
		return nil, errors.New("helm-release[] may not be set for Helm deployment events")
	}
	if len(event.TerraformRuns) == 0 {
		return nil, errors.New("terraform-runs[] may not be empty")
	}

	for idx, runInfo := range event.TerraformRuns {
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

		//Terraform will only show the change_summary if the operation completes successfully
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
// TranslationHandler for SNow

type terraformDeploymentToSNowTranslator struct {
}

func (t *terraformDeploymentToSNowTranslator) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}

func (t *terraformDeploymentToSNowTranslator) PluginTypeID() string {
	return "terraform-deployment-from-concourse.v1->terraform-deployment-to-servicenow.v1"
}

func (t *terraformDeploymentToSNowTranslator) TranslatePayload(payload []byte) ([]byte, error) {
	//TODO: stub
	return nil, errors.New("unimplemented")
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for SNow

type terraformDeploymentToSNowDeliverer struct {
}

func (d *terraformDeploymentToSNowDeliverer) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}

func (d *terraformDeploymentToSNowDeliverer) PluginTypeID() string {
	return "terraform-deployment-to-servicenow.v1"
}

func (d *terraformDeploymentToSNowDeliverer) DeliverPayload(payload []byte) (*tenso.DeliveryLog, error) {
	//TODO: stub
	return nil, errors.New("unimplemented")
}
