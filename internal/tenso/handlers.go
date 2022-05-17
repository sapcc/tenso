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

import "github.com/gophercloud/gophercloud"

//ValidationHandler is an object that validates incoming payloads of a specific
//payload type.
type ValidationHandler interface {
	//Init will be called at least once during startup if this ValidationHandler
	//is enabled in the configuration.
	//
	//A (ProviderClient, EndpointOpts) pair is provided for handlers that need to
	//talk to OpenStack. During unit tests, (nil, nil) will be provided instead.
	Init(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) error

	PayloadType() string
	ValidatePayload(payload []byte) error
}

//TranslationHandler is an object that can translate payloads from one specific
//payload type into a different payload type.
type TranslationHandler interface {
	//Init will be called at least once during startup if this TranslationHandler
	//is enabled in the configuration.
	//
	//A (ProviderClient, EndpointOpts) pair is provided for handlers that need to
	//talk to OpenStack. During unit tests, (nil, {}) will be provided instead.
	Init(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) error

	SourcePayloadType() string
	TargetPayloadType() string
	TranslatePayload(payload []byte) ([]byte, error)
}

//DeliveryHandler is an object that can deliver payloads of one specific
//payload type to a target in some way.
type DeliveryHandler interface {
	//Init will be called at least once during startup if this DeliveryHandler
	//is enabled in the configuration.
	//
	//A (ProviderClient, EndpointOpts) pair is provided for handlers that need to
	//talk to OpenStack. During unit tests, (nil, nil) will be provided instead.
	Init(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) error

	PayloadType() string
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

//Route describes a complete delivery path for events: An event gets submitted
//to us with an initial payload type, gets translated into a different payload
//type, and then the translated payload gets delivered.
type Route struct {
	SourcePayloadType  string
	TargetPayloadType  string
	ValidationHandler  ValidationHandler
	TranslationHandler TranslationHandler
	DeliveryHandler    DeliveryHandler
}
