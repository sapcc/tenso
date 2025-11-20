// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package handlers_test

import (
	"os"
	"testing"

	"github.com/sapcc/go-bits/assert"
	"github.com/sapcc/go-bits/must"

	"github.com/sapcc/tenso/internal/test"
)

func TestAWXWorkflowValidationAndConversionToSNow(t *testing.T) {
	t.Setenv("TENSO_SERVICENOW_MAPPING_CONFIG_PATH", "fixtures/servicenow-mapping-config.yaml")

	s := test.NewSetup(t,
		test.WithRoute("infra-workflow-from-awx.v1 -> infra-workflow-to-servicenow.v1"),
	)
	vh := s.Config.EnabledRoutes[0].ValidationHandler
	th := s.Config.EnabledRoutes[0].TranslationHandler

	sourcePayloadBytes := must.ReturnT(os.ReadFile("fixtures/infra-workflow-from-awx.v1.good.json"))(t)
	payloadInfo := must.ReturnT(vh.ValidatePayload(sourcePayloadBytes))(t)
	assert.Equal(t, payloadInfo.Description, "ESX upgrade, qa-de-1a, node002-bb091.cc.qa-de-1.cloud.sap")

	targetPayloadBytes := must.ReturnT(th.TranslatePayload(sourcePayloadBytes, nil))(t)
	expectTranslatedPayload(t, targetPayloadBytes, "fixtures/infra-workflow-to-servicenow.v1.good.json")
}
