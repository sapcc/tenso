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
	"database/sql"
	"net/http"
	"os"
	"time"

	"github.com/dlmiddlecote/sqlstats"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/sapcc/go-api-declarations/bininfo"
	"github.com/sapcc/go-bits/gopherpolicy"
	"github.com/sapcc/go-bits/httpapi"
	"github.com/sapcc/go-bits/httpext"
	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/go-bits/must"
	"github.com/sapcc/go-bits/osext"
	"gopkg.in/gorp.v2"

	"github.com/sapcc/tenso/internal/api"
	_ "github.com/sapcc/tenso/internal/handlers" //must be imported to register the handler implementations
	"github.com/sapcc/tenso/internal/tasks"
	"github.com/sapcc/tenso/internal/tenso"
)

func main() {
	commandWord := ""
	if len(os.Args) == 2 {
		commandWord = os.Args[1]
		bininfo.SetTaskName(commandWord)
	}

	logg.ShowDebug = osext.GetenvBool("TENSO_DEBUG")

	wrap := httpext.WrapTransport(&http.DefaultTransport)
	wrap.SetInsecureSkipVerify(osext.GetenvBool("TENSO_INSECURE")) //for debugging with mitmproxy etc. (DO NOT SET IN PRODUCTION)
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

	//wire up HTTP handlers
	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"HEAD", "GET", "POST", "PUT", "DELETE"},
		AllowedHeaders: []string{"Content-Type", "User-Agent", "X-Auth-Token", "Authorization"},
	})
	handler := httpapi.Compose(
		api.NewAPI(cfg, db, &tv),
		httpapi.HealthCheckAPI{SkipRequestLog: true},
		httpapi.WithGlobalMiddleware(corsMiddleware.Handler),
	)
	http.Handle("/", handler)
	http.Handle("/metrics", promhttp.Handler())

	//start HTTP server
	apiListenAddress := osext.GetenvOrDefault("TENSO_API_LISTEN_ADDRESS", ":8080")
	err = httpext.ListenAndServeContext(ctx, apiListenAddress, nil)
	if err != nil {
		logg.Fatal("error returned from httpext.ListenAndServeContext(): %s", err.Error())
	}
}

func runWorker(cfg tenso.Configuration, db *gorp.DbMap) {
	ctx := httpext.ContextWithSIGINT(context.Background(), 10*time.Second)

	//start worker loops (we have a budget of 16 DB connections, which we
	//distribute between converting and delivering with some headroom to spare)
	c := tasks.NewContext(cfg, db)
	goQueuedJobLoop(ctx, 7, c.PollForPendingConversions)
	goQueuedJobLoop(ctx, 7, c.PollForPendingDeliveries)
	go cronJobLoop(5*time.Minute, c.CollectGarbage)

	//wire up HTTP handlers for Prometheus metrics and health check
	handler := httpapi.Compose(httpapi.HealthCheckAPI{SkipRequestLog: true})
	http.Handle("/", handler)
	http.Handle("/metrics", promhttp.Handler())

	//start HTTP server
	listenAddress := osext.GetenvOrDefault("TENSO_WORKER_LISTEN_ADDRESS", ":8080")
	err := httpext.ListenAndServeContext(ctx, listenAddress, nil)
	if err != nil {
		logg.Fatal("error returned from httpext.ListenAndServeContext(): %s", err.Error())
	}
}

// Execute a task repeatedly, but slow down when sql.ErrNoRows is returned by it.
// (Tasks use this error value to indicate that nothing needs scraping, so we
// can back off a bit to avoid useless database load.)
func goQueuedJobLoop(ctx context.Context, numGoroutines int, poll tasks.JobPoller) {
	ch := make(chan tasks.Job) //unbuffered!

	//one goroutine to select tasks from the DB
	go func(ch chan<- tasks.Job) {
		for ctx.Err() == nil {
			job, err := poll()
			switch err {
			case nil:
				ch <- job
			case sql.ErrNoRows:
				//no jobs waiting right now - slow down a bit to avoid useless DB load
				time.Sleep(3 * time.Second)
			default:
				logg.Error(err.Error())
			}
		}

		//`ctx` has expired -> tell workers to shutdown
		close(ch)
	}(ch)

	//multiple goroutines to execute tasks
	//
	//We use `numGoroutines-1` here since we already have spawned one goroutine
	//for the polling above.
	for i := 0; i < numGoroutines-1; i++ {
		go func(ch <-chan tasks.Job) {
			for job := range ch {
				err := job.Execute()
				if err != nil {
					logg.Error(err.Error())
				}
			}
		}(ch)
	}
}

// Execute a task repeatedly, in set intervals. Unlike queuedJobLoop(), this
// does not change pace when errors are returned.
func cronJobLoop(interval time.Duration, task func() error) {
	for {
		err := task()
		if err != nil {
			logg.Error(err.Error())
		}
		time.Sleep(interval)
	}
}
