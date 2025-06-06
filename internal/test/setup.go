// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package test

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-gorp/gorp/v3"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sapcc/go-bits/easypg"
	"github.com/sapcc/go-bits/httpapi"
	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/go-bits/mock"
	"github.com/sapcc/go-bits/osext"

	"github.com/sapcc/tenso/internal/api"
	"github.com/sapcc/tenso/internal/tasks"
	"github.com/sapcc/tenso/internal/tenso"
)

type setupParams struct {
	RouteSpecs      []string
	WithAPI         bool
	WithTaskContext bool
}

// WithAPI is a SetupOption that provides a http.Handler with the Tenso API.
func WithAPI(params *setupParams) {
	params.WithAPI = true
}

// WithTaskContext is a SetupOption that provides a tasks.Context object for testing worker tasks.
func WithTaskContext(params *setupParams) {
	params.WithTaskContext = true
}

// WithRoute is a SetupOption that adds a route to the configuration.
func WithRoute(route string) SetupOption {
	return func(params *setupParams) {
		params.RouteSpecs = append(params.RouteSpecs, route)
	}
}

// SetupOption is an option that can be given to NewSetup().
type SetupOption func(*setupParams)

// Setup contains all the pieces that are needed for most tests.
type Setup struct {
	// fields that are always set
	Clock    *mock.Clock
	Config   tenso.Configuration
	DB       *gorp.DbMap
	Ctx      context.Context //nolint: containedctx  // only used in tests
	Registry *prometheus.Registry
	// fields that are set if WithAPI is included
	Validator *mock.Validator[*mock.Enforcer]
	Handler   http.Handler
	// fields that are set if WithTaskContext is included
	TaskContext *tasks.Context
}

// NewSetup prepares most or all pieces of Tenso for a test.
func NewSetup(t *testing.T, opts ...SetupOption) Setup {
	t.Helper()
	logg.ShowDebug = osext.GetenvBool("TENSO_DEBUG")
	var params setupParams
	for _, option := range opts {
		option(&params)
	}

	// connect to DB
	db := tenso.InitORM(easypg.ConnectForTest(t, tenso.DBConfiguration(),
		easypg.ClearTables("pending_deliveries", "events", "users"),
		easypg.ResetPrimaryKeys("events", "users"),
	))

	// build configuration
	routes, err := tenso.BuildRoutes(t.Context(), params.RouteSpecs, nil, gophercloud.EndpointOpts{})
	Must(t, err)
	s := Setup{
		Clock: mock.NewClock(),
		Config: tenso.Configuration{
			EnabledRoutes: routes,
		},
		Ctx:      t.Context(),
		DB:       db,
		Registry: prometheus.NewPedanticRegistry(),
	}

	// satisfy additional requests
	if params.WithAPI {
		s.Validator = mock.NewValidator(mock.NewEnforcer(), map[string]string{
			"user_name":        "testusername",
			"user_id":          "testuserid",
			"user_domain_name": "testdomainname",
		})
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
