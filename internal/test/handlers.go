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
	"encoding/json"

	"github.com/sapcc/tenso/internal/tenso"
)

//For tests, we define the payload types "test-foo.v1" and "test-bar.v1". The
//foo type can be ingested only, and the bar type can be delivered only.
//Payloads for "test-foo.v1" must be JSON documents like {"foo":<integer>}, and
//analogously for "test-bar.v1". Conversion from foo to bar payloads just
//renames the field, the value remains the same.

func init() {
	tenso.RegisterValidationHandler(&fooValidationHandler{})
	tenso.RegisterTranslationHandler(&fooBarTranslationHandler{})
	tenso.RegisterDeliveryHandler(&barDeliveryHandler{})
}

type fooPayload struct {
	Value int `json:"foo"`
}

type barPayload struct {
	Value int `json:"bar"`
}

type fooValidationHandler struct{}

func (h *fooValidationHandler) Init() error         { return nil }
func (h *fooValidationHandler) PayloadType() string { return "test-foo.v1" }
func (h *fooValidationHandler) ValidatePayload(payload []byte) error {
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var val fooPayload
	return dec.Decode(&val)
}

type fooBarTranslationHandler struct{}

func (h *fooBarTranslationHandler) Init() error               { return nil }
func (h *fooBarTranslationHandler) SourcePayloadType() string { return "test-foo.v1" }
func (h *fooBarTranslationHandler) TargetPayloadType() string { return "test-bar.v1" }
func (h *fooBarTranslationHandler) TranslatePayload(payload []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var src fooPayload
	err := dec.Decode(&src)
	if err != nil {
		return nil, err
	}
	return json.Marshal(barPayload(src))
}

type barDeliveryHandler struct{}

func (h *barDeliveryHandler) Init() error                         { return nil }
func (h *barDeliveryHandler) PayloadType() string                 { return "test-bar.v1" }
func (h *barDeliveryHandler) DeliverPayload(payload []byte) error { return nil } //TODO stub
