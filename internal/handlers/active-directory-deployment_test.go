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
	"fmt"
	"os"
	"testing"

	"github.com/sapcc/go-bits/assert"

	"github.com/sapcc/tenso/internal/test"
)

func TestActiveDirectoryDeploymentValidationSuccess(t *testing.T) {
	// we will not be using this, but we need some config for the DeliveryHandler for the test.Setup() to go through
	t.Setenv("TENSO_SERVICENOW_CREATE_CHANGE_URL", "http://www.example.com")
	// this one we actually need
	t.Setenv("TENSO_SERVICENOW_MAPPING_CONFIG_PATH", "fixtures/servicenow-mapping-config.yaml")

	for _, eventFormat := range []string{"v1", "v2"} {
		t.Logf("-- testing event format %s", eventFormat)

		s := test.NewSetup(t,
			test.WithRoute(fmt.Sprintf(
				"active-directory-deployment-from-concourse.%[1]s -> active-directory-deployment-to-servicenow.v1",
				eventFormat,
			)),
		)
		vh := s.Config.EnabledRoutes[0].ValidationHandler

		testCases := []string{
			fmt.Sprintf("fixtures/active-directory-deployment-from-concourse.%s.dev.json", eventFormat),
			fmt.Sprintf("fixtures/active-directory-deployment-from-concourse.%s.failed.json", eventFormat),
		}

		for _, tc := range testCases {
			sourcePayloadBytes, err := os.ReadFile(tc)
			test.Must(t, err)
			payloadInfo, err := vh.ValidatePayload(sourcePayloadBytes)
			test.Must(t, err)
			assert.DeepEqual(t, "event description",
				payloadInfo.Description,
				"core/active-directory: deploy AD to ad-dev.example.sap",
			)
		}
	}
}

// TODO test validation errors

func TestActiveDirectoryDeploymentConversionToSNow(t *testing.T) {
	// we will not be using this, but we need some config for the DeliveryHandler for the test.Setup() to go through
	t.Setenv("TENSO_SERVICENOW_CREATE_CHANGE_URL", "http://www.example.com")
	// this one we actually need
	t.Setenv("TENSO_SERVICENOW_MAPPING_CONFIG_PATH", "fixtures/servicenow-mapping-config.yaml")

	for _, eventFormat := range []string{"v1", "v2"} {
		t.Logf("-- testing event format %s", eventFormat)

		s := test.NewSetup(t,
			test.WithRoute(fmt.Sprintf(
				"active-directory-deployment-from-concourse.%s -> active-directory-deployment-to-servicenow.v1",
				eventFormat,
			)),
		)
		th := s.Config.EnabledRoutes[0].TranslationHandler

		sourcePayloadBytes, err := os.ReadFile(fmt.Sprintf("fixtures/active-directory-deployment-from-concourse.%s.dev.json", eventFormat))
		test.Must(t, err)
		targetPayloadBytes, err := th.TranslatePayload(sourcePayloadBytes, nil)
		test.Must(t, err)
		assert.JSONFixtureFile("fixtures/active-directory-deployment-to-servicenow.v1.dev.json").
			AssertResponseBody(t, "translated payload", targetPayloadBytes)
	}
}
