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

package handlers_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/sapcc/go-bits/assert"

	_ "github.com/sapcc/tenso/internal/handlers"
	"github.com/sapcc/tenso/internal/test"
)

func TestHelmDeploymentValidationSuccess(t *testing.T) {
	//we will not be using this, but we need some config for the DeliveryHandler for the test.Setup() to go through
	os.Setenv("TENSO_HELM_DEPLOYMENT_LOGSTASH_HOST", "localhost:1")

	s := test.NewSetup(t,
		test.WithRoute("helm-deployment-from-concourse.v1 -> helm-deployment-to-elk.v1"),
	)
	vh := s.Config.EnabledRoutes[0].ValidationHandler

	testCases := []struct {
		ReleaseName         string
		ExpectedDescription string
	}{
		{
			ReleaseName:         "kube-system-metal",
			ExpectedDescription: "services/kube-system-metal: deploy kube-system-metal to qa-de-1",
		},
		{
			ReleaseName:         "swift",
			ExpectedDescription: "services/swift: deploy swift to qa-de-1 and swift-utils to qa-de-1",
		},
	}

	for _, tc := range testCases {
		sourcePayloadBytes, err := os.ReadFile(fmt.Sprintf("fixtures/helm-deployment-from-concourse.v1.%s.json", tc.ReleaseName))
		test.Must(t, err)
		payloadInfo, err := vh.ValidatePayload(sourcePayloadBytes)
		test.Must(t, err)
		assert.DeepEqual(t, "event description", payloadInfo.Description, tc.ExpectedDescription)
	}
}

//TODO test validation errors

func TestHelmDeploymentConversionToSNow(t *testing.T) {
	//we will not be using this, but we need some config for the DeliveryHandler for the test.Setup() to go through
	os.Setenv("TENSO_SERVICENOW_CREATE_CHANGE_URL", "http://www.example.com")
	os.Setenv("TENSO_SERVICENOW_TOKEN_URL", "http://www.example.com")
	os.Setenv("TENSO_SERVICENOW_USERNAME", "foo")
	os.Setenv("TENSO_SERVICENOW_PASSWORD", "bar")
	//this one we actually need
	os.Setenv("TENSO_SERVICENOW_MAPPING_CONFIG_PATH", "fixtures/servicenow-mapping-config.yaml")

	s := test.NewSetup(t,
		test.WithRoute("helm-deployment-from-concourse.v1 -> helm-deployment-to-servicenow.v1"),
	)
	th := s.Config.EnabledRoutes[0].TranslationHandler

	sourcePayloadBytes, err := os.ReadFile("fixtures/helm-deployment-from-concourse.v1.swift.json")
	test.Must(t, err)
	targetPayloadBytes, err := th.TranslatePayload(sourcePayloadBytes)
	test.Must(t, err)
	assert.JSONFixtureFile("fixtures/helm-deployment-to-servicenow.v1.swift.json").
		AssertResponseBody(t, "translated payload", targetPayloadBytes)
}
