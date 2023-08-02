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

import (
	"context"
	"fmt"
	"strings"

	"github.com/gophercloud/gophercloud"
	"github.com/sapcc/go-bits/pluggable"
)

// ValidationHandler is an object that validates incoming payloads of a specific
// payload type. The PluginTypeID must be equal to the payload type.
type ValidationHandler interface {
	pluggable.Plugin
	//Init will be called at least once during startup if this ValidationHandler
	//is enabled in the configuration.
	//
	//A (ProviderClient, EndpointOpts) pair is provided for handlers that need to
	//talk to OpenStack. During unit tests, (nil, nil) will be provided instead.
	Init(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) error

	ValidatePayload(payload []byte) (*PayloadInfo, error)
}

// PayloadInfo contains structured information about a payload. It is returned
// by ValidationHandler.ValidatePayload().
type PayloadInfo struct {
	//Description is a short summary of the event with this payload. It is used
	//to identify the event in log messages.
	Description string
}

// TranslationHandler is an object that can translate payloads from one specific
// payload type into a different payload type. The PluginTypeID must be equal to
// "$SOURCE_PAYLOAD_TYPE->$TARGET_PAYLOAD_TYPE".
type TranslationHandler interface {
	pluggable.Plugin
	//Init will be called at least once during startup if this TranslationHandler
	//is enabled in the configuration.
	//
	//A (ProviderClient, EndpointOpts) pair is provided for handlers that need to
	//talk to OpenStack. During unit tests, (nil, {}) will be provided instead.
	Init(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) error

	TranslatePayload(payload []byte) ([]byte, error)
}

// SourcePayloadTypeOf extracts the source payload type from h.PluginTypeID().
func SourcePayloadTypeOf(h TranslationHandler) string {
	fields := strings.Split(h.PluginTypeID(), "->")
	if len(fields) != 2 {
		panic(fmt.Sprintf("malformed PluginTypeID for %T: %q", h, h.PluginTypeID()))
	}
	return fields[0]
}

// TargetPayloadTypeOf extracts the source payload type from h.PluginTypeID().
func TargetPayloadTypeOf(h TranslationHandler) string {
	fields := strings.Split(h.PluginTypeID(), "->")
	if len(fields) != 2 {
		panic(fmt.Sprintf("malformed PluginTypeID for %T: %q", h, h.PluginTypeID()))
	}
	return fields[1]
}

// DeliveryHandler is an object that can deliver payloads of one specific
// payload type to a target in some way. The PluginTypeID must be equal to the
// payload type.
type DeliveryHandler interface {
	pluggable.Plugin
	//Init will be called at least once during startup if this DeliveryHandler
	//is enabled in the configuration.
	//
	//A (ProviderClient, EndpointOpts) pair is provided for handlers that need to
	//talk to OpenStack. During unit tests, (nil, nil) will be provided instead.
	Init(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) error

	DeliverPayload(ctx context.Context, payload []byte) (*DeliveryLog, error)
}

// DeliveryLog can be returned by DeliverPayload() to produce additional log
// messages, e.g. to report the ID of an object that was created in the target
// system.
type DeliveryLog struct {
	Message string
}

var (
	// ValidationHandlerRegistry is a pluggable.Registry for ValidationHandler implementations.
	ValidationHandlerRegistry pluggable.Registry[ValidationHandler]
	// TranslationHandlerRegistry is a pluggable.Registry for TranslationHandler implementations.
	TranslationHandlerRegistry pluggable.Registry[TranslationHandler]
	// DeliveryHandlerRegistry is a pluggable.Registry for DeliveryHandler implementations.
	DeliveryHandlerRegistry pluggable.Registry[DeliveryHandler]
)

// Route describes a complete delivery path for events: An event gets submitted
// to us with an initial payload type, gets translated into a different payload
// type, and then the translated payload gets delivered.
type Route struct {
	SourcePayloadType  string
	TargetPayloadType  string
	ValidationHandler  ValidationHandler
	TranslationHandler TranslationHandler
	DeliveryHandler    DeliveryHandler
}
