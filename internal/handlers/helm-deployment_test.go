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

	for _, releaseName := range []string{"kube-system-metal", "swift"} {
		sourcePayloadBytes, err := os.ReadFile(fmt.Sprintf("fixtures/helm-deployment-from-concourse.v1.%s.json", releaseName))
		test.Must(t, err)
		test.Must(t, vh.ValidatePayload(sourcePayloadBytes))
	}
}

//TODO test validation errors
