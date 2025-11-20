// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package handlers_test

import (
	"os"
	"testing"

	"github.com/sapcc/go-bits/assert"
	"github.com/sapcc/go-bits/must"

	"github.com/sapcc/tenso/internal/test"
)

func TestTerraformDeploymentValidationSuccess(t *testing.T) {
	t.Setenv("TENSO_SERVICENOW_MAPPING_CONFIG_PATH", "fixtures/servicenow-mapping-config.yaml")

	s := test.NewSetup(t,
		test.WithRoute("terraform-deployment-from-concourse.v1 -> terraform-deployment-to-servicenow.v1"),
	)
	vh := s.Config.EnabledRoutes[0].ValidationHandler

	sourcePayloadBytes := must.ReturnT(os.ReadFile("fixtures/terraform-deployment-from-concourse.v1.terragrunt-virtual-apod.json"))(t)
	payloadInfo := must.ReturnT(vh.ValidatePayload(sourcePayloadBytes))(t)
	assert.Equal(t, payloadInfo.Description, "services/terragrunt-virtual-apod: Terraform run for vnode4-v-qa-de-1")
}

// TODO test validation errors

func TestTerraformDeploymentConversionToSNow(t *testing.T) {
	t.Setenv("TENSO_SERVICENOW_MAPPING_CONFIG_PATH", "fixtures/servicenow-mapping-config.yaml")

	s := test.NewSetup(t,
		test.WithRoute("terraform-deployment-from-concourse.v1 -> terraform-deployment-to-servicenow.v1"),
	)
	th := s.Config.EnabledRoutes[0].TranslationHandler

	sourcePayloadBytes := must.ReturnT(os.ReadFile("fixtures/terraform-deployment-from-concourse.v1.terragrunt-virtual-apod.json"))(t)
	targetPayloadBytes := must.ReturnT(th.TranslatePayload(sourcePayloadBytes, nil))(t)
	expectTranslatedPayload(t, targetPayloadBytes, "fixtures/terraform-deployment-to-servicenow.v1.terragrunt-virtual-apod.json")
}
