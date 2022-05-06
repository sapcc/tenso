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

	"github.com/gorilla/mux"
	"github.com/sapcc/go-bits/sre"

	"github.com/sapcc/tenso/internal/tenso"
)

type API struct {
	cfg tenso.Configuration
	db  *tenso.DB
}

func NewAPI(cfg tenso.Configuration, db *tenso.DB) *API {
	a := &API{cfg, db}
	return a
}

//Handler generates a HTTP handler for all main API endpoints.
func (a *API) Handler() http.Handler {
	r := mux.NewRouter()
	r.Methods("POST").Path("/v1/events/new").HandlerFunc(a.handlePostNewEvent)
	//TODO: add API endpoints
	return sre.Instrument(r)
}

//HealthCheckHandler provides the GET /healthcheck endpoint.
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.URL.Path == "/healthcheck" && (r.Method == "GET" || r.Method == "HEAD") {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	} else {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}
}
