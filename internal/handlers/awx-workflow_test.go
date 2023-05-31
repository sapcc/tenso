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
	"os"
	"testing"

	"github.com/sapcc/go-bits/assert"

	"github.com/sapcc/tenso/internal/test"
)

func TestAWXWorkflowValidationAndConversionToSNow(t *testing.T) {
	//we will not be using this, but we need some config for the DeliveryHandler for the test.Setup() to go through
	t.Setenv("TENSO_SERVICENOW_CREATE_CHANGE_URL", "http://www.example.com")
	//this one we actually need
	t.Setenv("TENSO_SERVICENOW_MAPPING_CONFIG_PATH", "fixtures/servicenow-mapping-config.yaml")

	s := test.NewSetup(t,
		test.WithRoute("infra-workflow-from-awx.v1 -> infra-workflow-to-servicenow.v1"),
	)
	vh := s.Config.EnabledRoutes[0].ValidationHandler
	th := s.Config.EnabledRoutes[0].TranslationHandler

	sourcePayloadBytes, err := os.ReadFile("fixtures/infra-workflow-from-awx.v1.good.json")
	test.Must(t, err)
	payloadInfo, err := vh.ValidatePayload(sourcePayloadBytes)
	test.Must(t, err)
	assert.DeepEqual(t, "event description", payloadInfo.Description, "ESX upgrade, qa-de-1a, node002-bb091.cc.qa-de-1.cloud.sap")
	targetPayloadBytes, err := th.TranslatePayload(sourcePayloadBytes)
	test.Must(t, err)
	assert.JSONFixtureFile("fixtures/infra-workflow-to-servicenow.v1.good.json").
		AssertResponseBody(t, "translated payload", targetPayloadBytes)
}
