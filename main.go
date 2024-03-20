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

package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/dlmiddlecote/sqlstats"
	"github.com/go-gorp/gorp/v3"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/sapcc/go-api-declarations/bininfo"
	"github.com/sapcc/go-bits/gopherpolicy"
	"github.com/sapcc/go-bits/httpapi"
	"github.com/sapcc/go-bits/httpapi/pprofapi"
	"github.com/sapcc/go-bits/httpext"
	"github.com/sapcc/go-bits/jobloop"
	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/go-bits/must"
	"github.com/sapcc/go-bits/osext"
	"go.uber.org/automaxprocs/maxprocs"

	"github.com/sapcc/tenso/internal/api"
	_ "github.com/sapcc/tenso/internal/handlers" // must be imported to register the handler implementations
	"github.com/sapcc/tenso/internal/tasks"
	"github.com/sapcc/tenso/internal/tenso"
)

func main() {
	bininfo.HandleVersionArgument()

	commandWord := ""
	if len(os.Args) == 2 {
		commandWord = os.Args[1]
		bininfo.SetTaskName(commandWord)
	}

	logg.ShowDebug = osext.GetenvBool("TENSO_DEBUG")
	undoMaxprocs := must.Return(maxprocs.Set(maxprocs.Logger(logg.Debug)))
	defer undoMaxprocs()

	wrap := httpext.WrapTransport(&http.DefaultTransport)
	wrap.SetInsecureSkipVerify(osext.GetenvBool("TENSO_INSECURE")) // for debugging with mitmproxy etc. (DO NOT SET IN PRODUCTION)
	wrap.SetOverrideUserAgent(bininfo.Component(), bininfo.VersionOr("rolling"))

	cfg, provider, eo := tenso.ParseConfiguration()
	db := must.Return(tenso.InitDB(cfg.DatabaseURL))
	prometheus.MustRegister(sqlstats.NewStatsCollector("tenso", db.Db))

	switch commandWord {
	case "api":
		runAPI(cfg, db, provider, eo)
	case "worker":
		runWorker(cfg, db)
	default:
		logg.Fatal("usage: %s [api|worker]", os.Args[0])
	}
}

func runAPI(cfg tenso.Configuration, db *gorp.DbMap, provider *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) {
	ctx := httpext.ContextWithSIGINT(context.Background(), 10*time.Second)

	identityV3, err := openstack.NewIdentityV3(provider, eo)
	if err != nil {
		logg.Fatal("cannot find Keystone V3 API: " + err.Error())
	}
	tv := gopherpolicy.TokenValidator{
		IdentityV3: identityV3,
		Cacher:     gopherpolicy.InMemoryCacher(),
	}
	must.Succeed(tv.LoadPolicyFile(osext.MustGetenv("TENSO_OSLO_POLICY_PATH")))

	// wire up HTTP handlers
	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"HEAD", "GET", "POST", "PUT", "DELETE"},
		AllowedHeaders: []string{"Content-Type", "User-Agent", "X-Auth-Token", "Authorization"},
	})
	handler := httpapi.Compose(
		api.NewAPI(cfg, db, &tv),
		httpapi.HealthCheckAPI{SkipRequestLog: true},
		httpapi.WithGlobalMiddleware(corsMiddleware.Handler),
		pprofapi.API{IsAuthorized: pprofapi.IsRequestFromLocalhost},
	)
	mux := http.NewServeMux()
	mux.Handle("/", handler)
	mux.Handle("/metrics", promhttp.Handler())

	// start HTTP server
	apiListenAddress := osext.GetenvOrDefault("TENSO_API_LISTEN_ADDRESS", ":8080")
	must.Succeed(httpext.ListenAndServeContext(ctx, apiListenAddress, mux))
}

func runWorker(cfg tenso.Configuration, db *gorp.DbMap) {
	ctx := httpext.ContextWithSIGINT(context.Background(), 10*time.Second)

	// start worker loops (we have a budget of 16 DB connections, which we
	// distribute between converting and delivering with some headroom to spare)
	c := tasks.NewContext(cfg, db)
	go c.ConversionJob(nil).Run(ctx, jobloop.NumGoroutines(7))
	go c.DeliveryJob(nil).Run(ctx, jobloop.NumGoroutines(7))
	go c.GarbageCollectionJob(nil).Run(ctx)

	// wire up HTTP handlers for Prometheus metrics and health check
	handler := httpapi.Compose(
		httpapi.HealthCheckAPI{SkipRequestLog: true},
		pprofapi.API{IsAuthorized: pprofapi.IsRequestFromLocalhost},
	)
	mux := http.NewServeMux()
	mux.Handle("/", handler)
	mux.Handle("/metrics", promhttp.Handler())

	// start HTTP server
	listenAddress := osext.GetenvOrDefault("TENSO_WORKER_LISTEN_ADDRESS", ":8080")
	must.Succeed(httpext.ListenAndServeContext(ctx, listenAddress, mux))
}
