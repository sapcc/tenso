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

package synthetic

import (
	_ "embed"
	"fmt"
)

//go:embed helm-deployment-from-concourse.v1.json
var helmV1Payload []byte

//go:embed infra-workflow-from-awx.v1.json
var infraWorkflowV1Payload []byte

// Event returns a synthetic event for the given payload type, or an error if
// no synthetic event exists for this payload type. Synthetic events are
// intended to be used by cloud admins to test conversions and delivery paths.
func Event(payloadType string) ([]byte, error) {
	switch payloadType {
	case "helm-deployment-from-concourse.v1":
		return helmV1Payload, nil
	case "infra-workflow-from-awx.v1":
		return infraWorkflowV1Payload, nil
	default:
		return nil, fmt.Errorf("no synthetic event available for payload type %q", payloadType)
	}
}
