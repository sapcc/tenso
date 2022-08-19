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
	"time"

	"github.com/gorilla/mux"
	"github.com/sapcc/go-bits/gopherpolicy"
	"gopkg.in/gorp.v2"

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
}
