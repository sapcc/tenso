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

package handlers_test

import (
	"os"
	"testing"

	"github.com/sapcc/go-bits/assert"

	"github.com/sapcc/tenso/internal/test"
)

func TestTerraformDeploymentValidationSuccess(t *testing.T) {
	//we will not be using this, but we need some config for the DeliveryHandler for the test.Setup() to go through
	t.Setenv("TENSO_SERVICENOW_CREATE_CHANGE_URL", "http://www.example.com")
	//this one we actually need
	t.Setenv("TENSO_SERVICENOW_MAPPING_CONFIG_PATH", "fixtures/servicenow-mapping-config.yaml")

	s := test.NewSetup(t,
		test.WithRoute("terraform-deployment-from-concourse.v1 -> terraform-deployment-to-servicenow.v1"),
	)
	vh := s.Config.EnabledRoutes[0].ValidationHandler

	sourcePayloadBytes, err := os.ReadFile("fixtures/terraform-deployment-from-concourse.v1.terragrunt-virtual-apod.json")
	test.Must(t, err)
	payloadInfo, err := vh.ValidatePayload(sourcePayloadBytes)
	test.Must(t, err)
	assert.DeepEqual(t, "event description",
		payloadInfo.Description,
		"services/terragrunt-virtual-apod: Terraform run for vnode4-v-qa-de-1",
	)
}

//TODO test validation errors

func TestTerraformDeploymentConversionToSNow(t *testing.T) {
	//we will not be using this, but we need some config for the DeliveryHandler for the test.Setup() to go through
	t.Setenv("TENSO_SERVICENOW_CREATE_CHANGE_URL", "http://www.example.com")
	//this one we actually need
	t.Setenv("TENSO_SERVICENOW_MAPPING_CONFIG_PATH", "fixtures/servicenow-mapping-config.yaml")

	s := test.NewSetup(t,
		test.WithRoute("terraform-deployment-from-concourse.v1 -> terraform-deployment-to-servicenow.v1"),
	)
	th := s.Config.EnabledRoutes[0].TranslationHandler

	sourcePayloadBytes, err := os.ReadFile("fixtures/terraform-deployment-from-concourse.v1.terragrunt-virtual-apod.json")
	test.Must(t, err)
	targetPayloadBytes, err := th.TranslatePayload(sourcePayloadBytes)
	test.Must(t, err)
	assert.JSONFixtureFile("fixtures/terraform-deployment-to-servicenow.v1.terragrunt-virtual-apod.json").
		AssertResponseBody(t, "translated payload", targetPayloadBytes)
}
