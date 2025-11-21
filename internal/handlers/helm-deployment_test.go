// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package handlers_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/sapcc/go-bits/assert"
	"github.com/sapcc/go-bits/must"

	_ "github.com/sapcc/tenso/internal/handlers"
	"github.com/sapcc/tenso/internal/test"
)

func TestHelmDeploymentValidationSuccess(t *testing.T) {
	// we will not be using this, but we need some config for the DeliveryHandler for the test.Setup() to go through
	t.Setenv("TENSO_HELM_DEPLOYMENT_LOGSTASH_HOST", "localhost:1")

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
		sourcePayloadBytes := must.ReturnT(os.ReadFile(fmt.Sprintf("fixtures/helm-deployment-from-concourse.v1.%s.json", tc.ReleaseName)))(t)
		payloadInfo := must.ReturnT(vh.ValidatePayload(sourcePayloadBytes))(t)
		assert.Equal(t, payloadInfo.Description, tc.ExpectedDescription)
	}
}

// TODO test validation errors

func TestHelmDeploymentConversionToSNow(t *testing.T) {
	t.Setenv("TENSO_SERVICENOW_MAPPING_CONFIG_PATH", "fixtures/servicenow-mapping-config.json")

	s := test.NewSetup(t,
		test.WithRoute("helm-deployment-from-concourse.v1 -> helm-deployment-to-servicenow.v1"),
	)
	th := s.Config.EnabledRoutes[0].TranslationHandler

	sourcePayloadBytes := must.ReturnT(os.ReadFile("fixtures/helm-deployment-from-concourse.v1.swift.json"))(t)
	targetPayloadBytes := must.ReturnT(th.TranslatePayload(sourcePayloadBytes, nil))(t)
	expectTranslatedPayload(t, targetPayloadBytes, "fixtures/helm-deployment-to-servicenow.v1.swift.json")
}
