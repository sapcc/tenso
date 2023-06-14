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

package handlers

import (
	"github.com/gophercloud/gophercloud"

	"github.com/sapcc/tenso/internal/tenso"
)

func init() {
	tenso.TranslationHandlerRegistry.Add(func() tenso.TranslationHandler {
		return &dummyTranslator{"helm-deployment-from-concourse.v1->helm-deployment-to-elk.v1"}
	})
	tenso.TranslationHandlerRegistry.Add(func() tenso.TranslationHandler {
		return &dummyTranslator{"helm-deployment-from-concourse.v1->helm-deployment-to-swift.v1"}
	})
	tenso.TranslationHandlerRegistry.Add(func() tenso.TranslationHandler {
		return &dummyTranslator{"infra-workflow-from-awx.v1->infra-workflow-to-swift.v1"}
	})
	tenso.TranslationHandlerRegistry.Add(func() tenso.TranslationHandler {
		return &dummyTranslator{"terraform-deployment-from-concourse.v1->terraform-deployment-to-swift.v1"}
	})
}

// dummyTranslator is a tenso.TranslationHandler for no-op translations.
type dummyTranslator struct {
	pluginTypeID string
}

func (h *dummyTranslator) PluginTypeID() string {
	return h.pluginTypeID
}

func (h *dummyTranslator) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}

func (h *dummyTranslator) TranslatePayload(payload []byte) ([]byte, error) {
	return payload, nil
}
