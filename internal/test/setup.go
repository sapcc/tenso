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

package test

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/sapcc/go-bits/httpapi"
	"github.com/sapcc/go-bits/logg"

	"github.com/sapcc/tenso/internal/api"
	"github.com/sapcc/tenso/internal/tasks"
	"github.com/sapcc/tenso/internal/tenso"
)

type setupParams struct {
	RouteSpecs      []string
	WithAPI         bool
	WithTaskContext bool
}

//WithAPI is a SetupOption that provides a http.Handler with the Tenso API.
func WithAPI(params *setupParams) {
	params.WithAPI = true
}

//WithTaskContext is a SetupOption that provides a tasks.Context object for testing worker tasks.
func WithTaskContext(params *setupParams) {
	params.WithTaskContext = true
}

//WithRoute is a SetupOption that adds a route to the configuration.
func WithRoute(route string) SetupOption {
	return func(params *setupParams) {
		params.RouteSpecs = append(params.RouteSpecs, route)
	}
}

//SetupOption is an option that can be given to NewSetup().
type SetupOption func(*setupParams)

//Setup contains all the pieces that are needed for most tests.
type Setup struct {
	//fields that are always set
	Clock  *Clock
	Config tenso.Configuration
	DB     *tenso.DB
	//fields that are set if WithAPI is included
	Validator *MockValidator
	Handler   http.Handler
	//fields that are set if WithTaskContext is included
	TaskContext *tasks.Context
}

//NewSetup prepares most or all pieces of Tenso for a test.
func NewSetup(t *testing.T, opts ...SetupOption) Setup {
	t.Helper()
	logg.ShowDebug = tenso.ParseBool(os.Getenv("TENSO_DEBUG"))
	var params setupParams
	for _, option := range opts {
		option(&params)
	}

	//connect to DB
	postgresURL := "postgres://postgres:postgres@localhost:54321/tenso?sslmode=disable"
	dbURL, err := url.Parse(postgresURL)
	Must(t, err)
	db, err := tenso.InitDB(*dbURL)
	if err != nil {
		t.Error(err)
		t.Log("Try prepending ./testing/with-postgres-db.sh to your command.")
		t.FailNow()
	}

	//wipe the DB clean if there are any leftovers from the previous test run
	//(the table order is chosen to respect all "ON DELETE RESTRICT" constraints)
	for _, tableName := range []string{"pending_deliveries", "events", "users"} {
		_, err := db.Exec("DELETE FROM " + tableName)
		Must(t, err)
	}

	//reset all primary key sequences for reproducible row IDs
	for _, tableName := range []string{"events", "users"} {
		nextID, err := db.SelectInt(fmt.Sprintf(
			"SELECT 1 + COALESCE(MAX(id), 0) FROM %s", tableName,
		))
		Must(t, err)

		query := fmt.Sprintf(`ALTER SEQUENCE %s_id_seq RESTART WITH %d`, tableName, nextID)
		_, err = db.Exec(query)
		Must(t, err)
	}

	//build configuration
	routes, err := tenso.BuildRoutes(params.RouteSpecs, nil, gophercloud.EndpointOpts{})
	Must(t, err)
	s := Setup{
		Clock: &Clock{},
		Config: tenso.Configuration{
			DatabaseURL:   *dbURL,
			EnabledRoutes: routes,
		},
		DB: db,
	}

	//satisfy additional requests
	if params.WithAPI {
		s.Validator = &MockValidator{}
		s.Handler = httpapi.Compose(
			api.NewAPI(s.Config, s.DB, s.Validator).OverrideTimeNow(s.Clock.Now),
			httpapi.WithoutLogging(),
		)
	}
	if params.WithTaskContext {
		s.TaskContext = tasks.NewContext(s.Config, s.DB).OverrideTimeNow(s.Clock.Now)
	}

	return s
}
