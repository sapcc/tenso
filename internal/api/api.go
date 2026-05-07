// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"regexp"
	"time"

	"github.com/go-gorp/gorp/v3"
	"github.com/gorilla/mux"
	"github.com/sapcc/go-bits/gopherpolicy"
	"github.com/sapcc/go-bits/osext"
	"github.com/sapcc/go-bits/regexpext"

	"github.com/sapcc/tenso/internal/tenso"
)

// API is a httpapi.API that serves the tenso API.
type API struct {
	Config    tenso.Configuration
	DB        *gorp.DbMap
	Validator gopherpolicy.Validator
	RegionRx  *regexp.Regexp
	timeNow   func() time.Time
}

// NewAPI creates an tenso API.
func NewAPI(cfg tenso.Configuration, db *gorp.DbMap, validator gopherpolicy.Validator) (*API, error) {
	regionRxEnvVar := "TENSO_REGION_REGEX"
	regionRxString, err := osext.NeedGetenv(regionRxEnvVar)
	if err != nil {
		return nil, err
	}
	regionRx, err := regexpext.BoundedRegexp(regionRxString).Regexp()
	if err != nil {
		return nil, fmt.Errorf("while compiling %s: %w", regionRxEnvVar, err)
	}
	return &API{cfg, db, validator, regionRx, time.Now}, nil
}

// OverrideTimeNow is used by unit tests to inject a mock clock.
func (a *API) OverrideTimeNow(now func() time.Time) *API {
	a.timeNow = now
	return a
}

// AddTo implements the httpapi.API interface.
func (a *API) AddTo(r *mux.Router) {
	r.Methods("POST").Path("/v1/events/new").HandlerFunc(a.handlePostNewEvent)
	r.Methods("POST").Path("/v1/events/synthetic").HandlerFunc(a.handlePostSyntheticEvent)
}
