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

package tenso

//ValidationHandler is an object that validates an incoming payload format.
type ValidationHandler interface {
	//Init will be called at least once during startup if this ValidationHandler
	//is enabled in the configuration.
	Init() error

	PayloadFormat() string
	ValidatePayload(payload []byte) error
}

//TranslationHandler is an object that can translate payloads from one specific
//format into a different format.
type TranslationHandler interface {
	//Init will be called at least once during startup if this TranslationHandler
	//is enabled in the configuration.
	Init() error

	SourcePayloadFormat() string
	TargetPayloadFormat() string
	TranslatePayload(payload []byte) ([]byte, error)
}

//DeliveryHandler is an object that can deliver payloads that are available in
//one specific format to a target in some way.
type DeliveryHandler interface {
	//Init will be called at least once during startup if this DeliveryHandler
	//is enabled in the configuration.
	Init() error

	PayloadFormat() string
	DeliverPayload(payload []byte) error
}

var (
	allValidationHandlers  []ValidationHandler
	allTranslationHandlers []TranslationHandler
	allDeliveryHandlers    []DeliveryHandler
)

//RegisterValidationHandler adds a ValidationHandler instance to the global
//lookup table.
func RegisterValidationHandler(h ValidationHandler) {
	allValidationHandlers = append(allValidationHandlers, h)
}

//RegisterTranslationHandler adds a TranslationHandler instance to the global
//lookup table.
func RegisterTranslationHandler(h TranslationHandler) {
	allTranslationHandlers = append(allTranslationHandlers, h)
}

//RegisterDeliveryHandler adds a DeliveryHandler instance to the global lookup
//table.
func RegisterDeliveryHandler(h DeliveryHandler) {
	allDeliveryHandlers = append(allDeliveryHandlers, h)
}

//DeliveryPath describes a complete delivery path for events: An event gets submitted to us in an initial format, gets translated into a different format, and then the translated payload gets delivered.
type Route struct {
	SourcePayloadFormat string
	TargetPayloadFormat string
	ValidationHandler   ValidationHandler
	TranslationHandler  TranslationHandler
	DeliveryHandler     DeliveryHandler
}
