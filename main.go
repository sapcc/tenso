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
	"crypto/tls"
	"net/http"
	"os"
	"time"

	"github.com/dlmiddlecote/sqlstats"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/sapcc/go-bits/gopherpolicy"
	"github.com/sapcc/go-bits/httpee"
	"github.com/sapcc/go-bits/logg"

	"github.com/sapcc/tenso/internal/api"
	_ "github.com/sapcc/tenso/internal/delivery" //must be imported to register the DeliveryHandlers
	"github.com/sapcc/tenso/internal/tenso"
	_ "github.com/sapcc/tenso/internal/translation" //must be imported to register the TranslationHandlers
	_ "github.com/sapcc/tenso/internal/validation"  //must be imported to register the ValidationHandlers
)

func main() {
	commandWord := ""
	if len(os.Args) == 2 {
		commandWord = os.Args[1]
		tenso.Component = "tenso-" + commandWord
	}

	logg.ShowDebug = tenso.ParseBool(os.Getenv("TENSO_DEBUG"))

	//The TENSO_INSECURE flag can be used to get Tenso to work through mitmproxy
	//(which is very useful for development and debugging). (It's very important
	//that this is not the standard "TENSO_DEBUG" variable. That one is meant to
	//be useful for production systems, where you definitely don't want to turn
	//off certificate verification.)
	if tenso.ParseBool(os.Getenv("TENSO_INSECURE")) {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
		http.DefaultClient.Transport = userAgentInjector{http.DefaultTransport}
	}

	cfg, provider, eo := tenso.ParseConfiguration()
	db, err := tenso.InitDB(cfg.DatabaseURL)
	must(err)
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

func runAPI(cfg tenso.Configuration, db *tenso.DB, provider *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) {
	ctx := httpee.ContextWithSIGINT(context.Background(), 10*time.Second)

	identityV3, err := openstack.NewIdentityV3(provider, eo)
	if err != nil {
		logg.Fatal("cannot find Keystone V3 API: " + err.Error())
	}
	tv := gopherpolicy.TokenValidator{
		IdentityV3: identityV3,
		Cacher:     gopherpolicy.InMemoryCacher(),
	}
	osloPolicyPath := os.Getenv("TENSO_OSLO_POLICY_PATH")
	if osloPolicyPath == "" {
		logg.Fatal("missing required environment variable: TENSO_OSLO_POLICY_PATH")
	}
	err = tv.LoadPolicyFile(osloPolicyPath)
	if err != nil {
		logg.Fatal("cannot load oslo.policy: " + err.Error())
	}

	//wire up HTTP handlers
	handler := api.NewAPI(cfg, db, &tv).Handler()
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
	err = httpee.ListenAndServeContext(ctx, apiListenAddress, nil)
	if err != nil {
		logg.Fatal("error returned from httpee.ListenAndServeContext(): %s", err.Error())
	}
}

//nolint:unparam
func runWorker(cfg tenso.Configuration, db *tenso.DB) {
	ctx := httpee.ContextWithSIGINT(context.Background(), 10*time.Second)

	//TODO start worker job loops

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

type userAgentInjector struct {
	Inner http.RoundTripper
}

//RoundTrip implements the http.RoundTripper interface.
func (uai userAgentInjector) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", tenso.Component+"/rolling")
	return uai.Inner.RoundTrip(req)
}
