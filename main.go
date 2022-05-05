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

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/sapcc/go-bits/httpee"
	"github.com/sapcc/go-bits/logg"

	"github.com/sapcc/tenso/internal/api"
	"github.com/sapcc/tenso/internal/tenso"
)

func main() {
	cfg := tenso.ParseConfiguration()
	db, err := tenso.InitDB(cfg.DatabaseURL)
	must(err)

	commandWord := ""
	if len(os.Args) == 2 {
		commandWord = os.Args[1]
	}
	switch commandWord {
	case "api":
		runAPI(cfg, db)
	case "worker":
		runWorker(cfg, db)
	default:
		logg.Fatal("usage: %s [api|worker]", os.Args[0])
	}
}

func runAPI(cfg tenso.Configuration, db *tenso.DB) {
	ctx := httpee.ContextWithSIGINT(context.Background(), 10*time.Second)

	//wire up HTTP handlers
	handler := api.Handler(cfg, db)
	handler = logg.Middleware{}.Wrap(handler)
	handler = cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"HEAD", "GET", "POST", "PUT", "DELETE"},
		AllowedHeaders: []string{"Content-Type", "User-Agent", "X-Auth-Token", "Authorization"},
	}).Handler(handler)
	http.Handle("/", handler)
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/healthcheck", api.HealthCheckHandler)

	//start HTTP server
	apiListenAddress := os.Getenv("TENSO_API_LISTEN_ADDRESS")
	if apiListenAddress == "" {
		apiListenAddress = ":8080"
	}
	logg.Info("listening on " + apiListenAddress)
	err := httpee.ListenAndServeContext(ctx, apiListenAddress, nil)
	if err != nil {
		logg.Fatal("error returned from httpee.ListenAndServeContext(): %s", err.Error())
	}
}

//nolint:unparam
func runWorker(cfg tenso.Configuration, db *tenso.DB) {
	ctx := httpee.ContextWithSIGINT(context.Background(), 10*time.Second)

	//start HTTP server for Prometheus metrics and health check
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/healthcheck", api.HealthCheckHandler)
	listenAddress := os.Getenv("TENSO_WORKER_LISTEN_ADDRESS")
	if listenAddress == "" {
		listenAddress = ":8080"
	}
	logg.Info("listening on " + listenAddress)
	err := httpee.ListenAndServeContext(ctx, listenAddress, nil)
	if err != nil {
		logg.Fatal("error returned from httpee.ListenAndServeContext(): %s", err.Error())
	}
}

func must(err error) {
	if err != nil {
		logg.Fatal(err.Error())
	}
}
