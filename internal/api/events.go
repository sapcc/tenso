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

package api

import (
	"fmt"
	"io"
	"net/http"

	"github.com/sapcc/go-bits/respondwith"
	"github.com/sapcc/go-bits/sre"

	"github.com/sapcc/tenso/internal/tenso"
)

const (
	maxIncomingPayloadBytes = 10 << 20 // 10 MiB
)

var (
	findOrCreateUserQuery = tenso.SimplifyWhitespaceInSQL(`
		INSERT INTO users (uuid, name, domain_name) VALUES ($1, $2, $3)
		ON CONFLICT (uuid) DO UPDATE SET name = EXCLUDED.name, domain_name = EXCLUDED.domain_name
		RETURNING id
	`)
)

func (a *API) handlePostNewEvent(w http.ResponseWriter, r *http.Request) {
	sre.IdentifyEndpoint(r, "/v1/events/new")
	requestTime := a.timeNow()

	//collect required query parameters
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

	//check authorization
	token := a.CheckToken(r)
	token.Context.Request = map[string]string{"target.payload_type": payloadType}
	for k, v := range token.Context.Auth {
		//HACK: without this, policy rules of the form "constant":%(variable)s will not work with auth variables
		//TODO: fix this issue upstream in goslo.policy
		token.Context.Request[k] = v
	}
	if !token.Require(w, "event:create") {
		return
	}

	//check that payload type is known
	var (
		validationHandler  tenso.ValidationHandler
		targetPayloadTypes []string
	)
	for _, route := range a.Config.EnabledRoutes {
		if route.SourcePayloadType == payloadType {
			targetPayloadTypes = append(targetPayloadTypes, route.TargetPayloadType)
			//NOTE: If there are multiple routes with the same SourcePayloadType,
			//they will have the same ValidationHandler, so it does not matter which
			//one we pick.
			validationHandler = route.ValidationHandler
		}
	}
	if validationHandler == nil {
		http.Error(w, fmt.Sprintf("cannot accept events with payload_type %q", payloadType), http.StatusBadRequest)
		return
	}

	//validate incoming payload
	payloadBytes, err := io.ReadAll(io.LimitReader(r.Body, maxIncomingPayloadBytes))
	if respondwith.ErrorText(w, err) {
		return
	}
	payloadInfo, err := validationHandler.ValidatePayload(payloadBytes)
	if err != nil {
		http.Error(w, "invalid event payload: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	//find or create user account
	userID, err := a.DB.SelectInt(findOrCreateUserQuery,
		token.UserUUID(), token.UserName(), token.UserDomainName(),
	)
	if respondwith.ErrorText(w, err) {
		return
	}

	//create DB records for this event
	tx, err := a.DB.Begin()
	if respondwith.ErrorText(w, err) {
		return
	}
	defer tenso.RollbackUnlessCommitted(tx)

	event := tenso.Event{
		CreatorID:   userID,
		CreatedAt:   requestTime,
		PayloadType: payloadType,
		Payload:     string(payloadBytes),
		Description: payloadInfo.Description,
	}
	err = tx.Insert(&event)
	if respondwith.ErrorText(w, err) {
		return
	}
	for _, targetPayloadType := range targetPayloadTypes {
		err = tx.Insert(&tenso.PendingDelivery{
			EventID:               event.ID,
			PayloadType:           targetPayloadType,
			Payload:               nil, //to be converted later
			ConvertedAt:           nil, //to be converted later
			FailedConversionCount: 0,
			FailedDeliveryCount:   0,
			NextConversionAt:      requestTime, //convert immediately
			NextDeliveryAt:        requestTime, //deliver immediately once converted
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
