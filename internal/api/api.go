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
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sapcc/go-bits/gopherpolicy"
	"github.com/sapcc/go-bits/logg"
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

func (a *API) CheckToken(r *http.Request) *gopherpolicy.Token {
	token := a.Validator.CheckToken(r)
	token.Context.Logger = logg.Debug
	logg.Debug("token has auth = %v", token.Context.Auth)
	logg.Debug("token has roles = %v", token.Context.Roles)
	return token
}
