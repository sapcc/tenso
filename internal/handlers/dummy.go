// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/gophercloud/gophercloud/v2"

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

func (h *dummyTranslator) Init(context.Context, *gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}

func (h *dummyTranslator) TranslatePayload(payload []byte, routingInfo map[string]string) ([]byte, error) {
	return payload, nil
}
