// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sapcc/go-bits/httpapi"
	"github.com/sapcc/go-bits/respondwith"
	"github.com/sapcc/go-bits/sqlext"

	"github.com/sapcc/tenso/internal/synthetic"
	"github.com/sapcc/tenso/internal/tenso"
)

const (
	maxIncomingPayloadBytes = 10 << 20 // 10 MiB
)

var (
	findOrCreateUserQuery = sqlext.SimplifyWhitespace(`
		INSERT INTO users (uuid, name, domain_name) VALUES ($1, $2, $3)
		ON CONFLICT (uuid) DO UPDATE SET name = EXCLUDED.name, domain_name = EXCLUDED.domain_name
		RETURNING id
	`)
)

func (a *API) handlePostNewEvent(w http.ResponseWriter, r *http.Request) {
	httpapi.IdentifyEndpoint(r, "/v1/events/new")
	getEventPayload := func(payloadType string) ([]byte, error) {
		return io.ReadAll(io.LimitReader(r.Body, maxIncomingPayloadBytes))
	}
	a.handlePostNewEventCommon(w, r, "event:create", getEventPayload)
}

func (a *API) handlePostSyntheticEvent(w http.ResponseWriter, r *http.Request) {
	httpapi.IdentifyEndpoint(r, "/v1/events/synthetic")
	a.handlePostNewEventCommon(w, r, "event:create_synthetic", synthetic.Event)
}

func (a *API) handlePostNewEventCommon(w http.ResponseWriter, r *http.Request, policyRule string, getPayload func(payloadType string) ([]byte, error)) {
	requestTime := a.timeNow()

	// collect required query parameters
	query := r.URL.Query()
	if len(query["payload_type"]) != 1 {
		http.Error(w, `need exactly one value for query parameter "payload_type"`, http.StatusBadRequest)
		return
	}
	payloadType := query.Get("payload_type")
	if !tenso.IsWellformedPayloadType(payloadType) {
		http.Error(w, `invalid value provided for query parameter "payload_type"`, http.StatusBadRequest)
		return
	}

	// check authorization
	token := a.Validator.CheckToken(r)
	token.Context.Request = map[string]string{"target.payload_type": payloadType}
	if !token.Require(w, policyRule) {
		return
	}

	// check that payload type is known
	var (
		validationHandler  tenso.ValidationHandler
		targetPayloadTypes []string
	)
	for _, route := range a.Config.EnabledRoutes {
		if route.SourcePayloadType == payloadType {
			targetPayloadTypes = append(targetPayloadTypes, route.TargetPayloadType)
			//NOTE: If there are multiple routes with the same SourcePayloadType,
			// they will have the same ValidationHandler, so it does not matter which
			// one we pick.
			validationHandler = route.ValidationHandler
		}
	}
	if validationHandler == nil {
		http.Error(w, fmt.Sprintf("cannot accept events with payload_type %q", payloadType), http.StatusBadRequest)
		return
	}

	// validate incoming payload
	payloadBytes, err := getPayload(payloadType)
	if respondwith.ErrorText(w, err) {
		return
	}
	payloadInfo, err := validationHandler.ValidatePayload(payloadBytes)
	if err != nil {
		http.Error(w, "invalid event payload: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	// parse headers
	routingInfo, err := parseRoutingInfo(r.Header.Get("X-Tenso-Routing-Info"))
	if err != nil {
		http.Error(w, "invalid routing info: "+err.Error(), http.StatusBadRequest)
		return
	}
	routingInfoJSON, err := json.Marshal(routingInfo)
	if respondwith.ErrorText(w, err) {
		return
	}

	// find or create user account
	userID, err := a.DB.SelectInt(findOrCreateUserQuery,
		token.UserUUID(), token.UserName(), token.UserDomainName(),
	)
	if respondwith.ErrorText(w, err) {
		return
	}

	// create DB records for this event
	tx, err := a.DB.Begin()
	if respondwith.ErrorText(w, err) {
		return
	}
	defer sqlext.RollbackUnlessCommitted(tx)

	event := tenso.Event{
		CreatorID:       userID,
		CreatedAt:       requestTime,
		PayloadType:     payloadType,
		Payload:         string(payloadBytes),
		Description:     payloadInfo.Description,
		RoutingInfoJSON: string(routingInfoJSON),
	}
	err = tx.Insert(&event)
	if respondwith.ErrorText(w, err) {
		return
	}
	for _, targetPayloadType := range targetPayloadTypes {
		err = tx.Insert(&tenso.PendingDelivery{
			EventID:               event.ID,
			PayloadType:           targetPayloadType,
			Payload:               nil, // to be converted later
			ConvertedAt:           nil, // to be converted later
			FailedConversionCount: 0,
			FailedDeliveryCount:   0,
			NextConversionAt:      requestTime, // convert immediately
			NextDeliveryAt:        requestTime, // deliver immediately once converted
		})
		if respondwith.ErrorText(w, err) {
			return
		}
	}
	err = tx.Commit()
	if respondwith.ErrorText(w, err) {
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// Parses the value from a X-Tenso-Routing-Info header.
//
// Example: "target=foobar, priority=42" -> {"target": "foobar", "priority": "42"}
func parseRoutingInfo(input string) (map[string]string, error) {
	result := make(map[string]string)
	for _, field := range strings.Split(input, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}

		key, value, ok := strings.Cut(field, "=")
		if !ok || key == "" || value == "" {
			return nil, fmt.Errorf(`expected a "key=value" pair, but found %q`, field)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if result[key] != "" {
			return nil, fmt.Errorf("multiple values for key %q", key)
		}
		result[key] = value
	}

	return result, nil
}
