// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/dlmiddlecote/sqlstats"
	"github.com/go-gorp/gorp/v3"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/sapcc/go-api-declarations/bininfo"
	"github.com/sapcc/go-bits/easypg"
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

	ctx := httpext.ContextWithSIGINT(context.Background(), 10*time.Second)
	cfg, provider, eo := tenso.ParseConfiguration(ctx)

	// initialize DB connection
	dbName := osext.GetenvOrDefault("TENSO_DB_NAME", "tenso")
	dbURL := must.Return(easypg.URLFrom(easypg.URLParts{
		HostName:          osext.GetenvOrDefault("TENSO_DB_HOSTNAME", "localhost"),
		Port:              osext.GetenvOrDefault("TENSO_DB_PORT", "5432"),
		UserName:          osext.GetenvOrDefault("TENSO_DB_USERNAME", "postgres"),
		Password:          os.Getenv("TENSO_DB_PASSWORD"),
		ConnectionOptions: os.Getenv("TENSO_DB_CONNECTION_OPTIONS"),
		DatabaseName:      dbName,
	}))
	dbConn := must.Return(easypg.Connect(dbURL, tenso.DBConfiguration()))
	prometheus.MustRegister(sqlstats.NewStatsCollector(dbName, dbConn))
	db := tenso.InitORM(dbConn)

	switch commandWord {
	case "api":
		runAPI(ctx, cfg, db, provider, eo)
	case "worker":
		runWorker(ctx, cfg, db)
	default:
		logg.Fatal("usage: %s [api|worker]", os.Args[0])
	}
}

func runAPI(ctx context.Context, cfg tenso.Configuration, db *gorp.DbMap, provider *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) {
	identityV3, err := openstack.NewIdentityV3(provider, eo)
	if err != nil {
		logg.Fatal("cannot find Keystone V3 API: " + err.Error())
	}
	tv := gopherpolicy.TokenValidator{
		IdentityV3: identityV3,
		Cacher:     gopherpolicy.InMemoryCacher(),
	}
	must.Succeed(tv.LoadPolicyFile(osext.MustGetenv("TENSO_OSLO_POLICY_PATH"), nil))

	// wire up HTTP handlers
	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"HEAD", "GET", "POST", "PUT", "DELETE"},
		AllowedHeaders: []string{"Content-Type", "User-Agent", "X-Auth-Token", "Authorization"},
	})
	handler := httpapi.Compose(
		api.NewAPI(cfg, db, &tv),
		httpapi.HealthCheckAPI{
			SkipRequestLog: true,
			Check: func() error {
				return db.Db.PingContext(ctx)
			},
		},
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

func runWorker(ctx context.Context, cfg tenso.Configuration, db *gorp.DbMap) {
	// start worker loops (we have a budget of 16 DB connections, which we
	// distribute between converting and delivering with some headroom to spare)
	c := tasks.NewContext(cfg, db)
	go c.ConversionJob(nil).Run(ctx, jobloop.NumGoroutines(7))
	go c.DeliveryJob(nil).Run(ctx, jobloop.NumGoroutines(7))
	go c.GarbageCollectionJob(nil).Run(ctx)

	// wire up HTTP handlers for Prometheus metrics and health check
	handler := httpapi.Compose(
		httpapi.HealthCheckAPI{
			SkipRequestLog: true,
			Check: func() error {
				return db.Db.PingContext(ctx)
			},
		},
		pprofapi.API{IsAuthorized: pprofapi.IsRequestFromLocalhost},
	)
	mux := http.NewServeMux()
	mux.Handle("/", handler)
	mux.Handle("/metrics", promhttp.Handler())

	// start HTTP server
	listenAddress := osext.GetenvOrDefault("TENSO_WORKER_LISTEN_ADDRESS", ":8080")
	must.Succeed(httpext.ListenAndServeContext(ctx, listenAddress, mux))
}
