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

package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"

	"github.com/sapcc/tenso/internal/tenso"
)

// For tests, we define the payload types "test-foo.v1" and "test-bar.v1". The
// foo type can be ingested only, and the bar type can be delivered only.
// Payloads for "test-foo.v1" must be JSON documents like {"foo":<integer>}, and
// analogously for "test-bar.v1". Conversion from foo to bar payloads just
// renames the field, the value remains the same.

func init() {
	tenso.ValidationHandlerRegistry.Add(func() tenso.ValidationHandler { return &testValidationHandler{"foo"} })
	tenso.TranslationHandlerRegistry.Add(func() tenso.TranslationHandler { return &testTranslationHandler{"foo", "bar"} })
	tenso.TranslationHandlerRegistry.Add(func() tenso.TranslationHandler { return &testTranslationHandler{"foo", "baz"} })
	tenso.DeliveryHandlerRegistry.Add(func() tenso.DeliveryHandler { return &testDeliveryHandler{"bar"} })
	tenso.DeliveryHandlerRegistry.Add(func() tenso.DeliveryHandler { return &testDeliveryHandler{"baz"} })
}

type testPayload struct {
	Event       string            `json:"event"`
	RoutingInfo map[string]string `json:"routing_info"`
	Value       int               `json:"value"`
}

func parseTestPayload(data []byte, expectedType string) (p testPayload, err error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	err = dec.Decode(&p)
	if err != nil {
		return testPayload{}, err
	}
	if p.Event != expectedType {
		return testPayload{}, fmt.Errorf("expected event = %q, but got %q", expectedType, p.Event)
	}
	return p, nil
}

type testValidationHandler struct {
	Type string
}

func (h *testValidationHandler) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}
func (h *testValidationHandler) PluginTypeID() string {
	return fmt.Sprintf("test-%s.v1", h.Type)
}

func (h *testValidationHandler) ValidatePayload(data []byte) (*tenso.PayloadInfo, error) {
	p, err := parseTestPayload(data, h.Type)
	if err != nil {
		return nil, err
	}
	return &tenso.PayloadInfo{
		Description: fmt.Sprintf("%s event with value %d", p.Event, p.Value),
	}, nil
}

type testTranslationHandler struct {
	SourceType string
	TargetType string
}

func (h *testTranslationHandler) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}
func (h *testTranslationHandler) PluginTypeID() string {
	return fmt.Sprintf("test-%s.v1->test-%s.v1", h.SourceType, h.TargetType)
}

func (h *testTranslationHandler) TranslatePayload(data []byte, routingInfo map[string]string) ([]byte, error) {
	p, err := parseTestPayload(data, h.SourceType)
	if err != nil {
		return nil, err
	}
	p.Event = h.TargetType
	p.RoutingInfo = routingInfo
	return json.Marshal(p)
}

type testDeliveryHandler struct {
	Type string
}

func (h *testDeliveryHandler) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}
func (h *testDeliveryHandler) PluginTypeID() string {
	return fmt.Sprintf("test-%s.v1", h.Type)
}

func (h *testDeliveryHandler) DeliverPayload(_ context.Context, data []byte, routingInfo map[string]string) (*tenso.DeliveryLog, error) {
	// We don't actually deliver anywhere, but by giving us an invalid payload, tests can "simulate" a delivery failure.
	_, err := parseTestPayload(data, h.Type)
	if err != nil {
		return nil, errors.New("simulating failed delivery because of invalid payload")
	}
	msg := fmt.Sprintf("success (routing info was: %v)", routingInfo)
	return &tenso.DeliveryLog{Message: msg}, nil
}
