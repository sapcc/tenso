// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"time"

	"github.com/go-gorp/gorp/v3"
	"github.com/gorilla/mux"
	"github.com/sapcc/go-bits/gopherpolicy"

	"github.com/sapcc/tenso/internal/tenso"
)

type API struct {
	Config    tenso.Configuration
	DB        *gorp.DbMap
	Validator gopherpolicy.Validator
	timeNow   func() time.Time
}

func NewAPI(cfg tenso.Configuration, db *gorp.DbMap, validator gopherpolicy.Validator) *API {
	return &API{cfg, db, validator, time.Now}
}

// OverrideTimeNow is used by unit tests to inject a mock clock.
func (a *API) OverrideTimeNow(now func() time.Time) *API {
	a.timeNow = now
	return a
}

// Handler implements the httpapi.API interface.
func (a *API) AddTo(r *mux.Router) {
	r.Methods("POST").Path("/v1/events/new").HandlerFunc(a.handlePostNewEvent)
	r.Methods("POST").Path("/v1/events/synthetic").HandlerFunc(a.handlePostSyntheticEvent)
}
