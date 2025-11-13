// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package handlers_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/sapcc/go-bits/assert"

	"github.com/sapcc/tenso/internal/test"
)

func TestActiveDirectoryDeploymentValidationSuccess(t *testing.T) {
	t.Setenv("TENSO_SERVICENOW_MAPPING_CONFIG_PATH", "fixtures/servicenow-mapping-config.json")

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
	t.Setenv("TENSO_SERVICENOW_MAPPING_CONFIG_PATH", "fixtures/servicenow-mapping-config.json")

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
