// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"os"
	"testing"

	"github.com/sapcc/go-bits/assert"

	"github.com/sapcc/tenso/internal/test"
)

func TestAWXWorkflowValidationAndConversionToSNow(t *testing.T) {
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
	targetPayloadBytes, err := th.TranslatePayload(sourcePayloadBytes, nil)
	test.Must(t, err)
	assert.JSONFixtureFile("fixtures/infra-workflow-to-servicenow.v1.good.json").
		AssertResponseBody(t, "translated payload", targetPayloadBytes)
}
