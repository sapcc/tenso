// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package synthetic

import (
	_ "embed"
	"fmt"
)

//go:embed helm-deployment-from-concourse.v1.json
var helmV1Payload []byte

//go:embed infra-workflow-from-awx.v1.json
var infraWorkflowV1Payload []byte

//go:embed terraform-deployment-from-concourse.v1.json
var terraformV1Payload []byte

//go:embed active-directory-deployment-from-concourse.v1.json
var activeDirectoryV1Payload []byte

//go:embed active-directory-deployment-from-concourse.v2.json
var activeDirectoryV2Payload []byte

// Event returns a synthetic event for the given payload type, or an error if
// no synthetic event exists for this payload type. Synthetic events are
// intended to be used by cloud admins to test conversions and delivery paths.
func Event(payloadType string) ([]byte, error) {
	switch payloadType {
	case "helm-deployment-from-concourse.v1":
		return helmV1Payload, nil
	case "infra-workflow-from-awx.v1":
		return infraWorkflowV1Payload, nil
	case "terraform-deployment-from-concourse.v1":
		return terraformV1Payload, nil
	case "active-directory-deployment-from-concourse.v1":
		return activeDirectoryV1Payload, nil
	case "active-directory-deployment-from-concourse.v2":
		return activeDirectoryV2Payload, nil
	default:
		return nil, fmt.Errorf("no synthetic event available for payload type %q", payloadType)
	}
}
