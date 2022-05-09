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

package tenso

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/sapcc/go-bits/logg"
)

//Configuration contains all configuration values that we collect from the environment.
type Configuration struct {
	DatabaseURL   url.URL
	EnabledRoutes []Route
}

var (
	routeSpecRx = regexp.MustCompile(`^([a-zA-Z0-9.-]+)\s*->\s*([a-zA-Z0-9.-]+)$`)
)

//ParseConfiguration obtains a tenso.Configuration instance from the
//corresponding environment variables. Aborts on error.
func ParseConfiguration() Configuration {
	cfg := Configuration{
		DatabaseURL: getDBURL(),
	}

	//parse routes
	for _, routeSpec := range strings.Split(os.Getenv("TENSO_ROUTES"), ",") {
		routeSpec = strings.TrimSpace(routeSpec)
		if routeSpec == "" {
			//be lenient e.g. when the list of routes has a trailing comma
			continue
		}

		match := routeSpecRx.FindStringSubmatch(routeSpec)
		if match == nil {
			logg.Fatal("route specification %q is invalid: syntax error", routeSpec)
		}
		route := Route{
			SourcePayloadType: match[1],
			TargetPayloadType: match[2],
		}

		//select validation handler
		for _, handler := range allValidationHandlers {
			if route.SourcePayloadType == handler.PayloadType() {
				route.ValidationHandler = handler
				break
			}
		}
		if route.ValidationHandler == nil {
			logg.Fatal("route specification %q is invalid: cannot validate %s",
				routeSpec, route.SourcePayloadType)
		}
		err := route.ValidationHandler.Init()
		if err != nil {
			logg.Fatal("while parsing route specification %q: cannot initialize validation for %s: %s",
				routeSpec, route.SourcePayloadType, err.Error())
		}

		//select translation handler
		for _, handler := range allTranslationHandlers {
			if route.SourcePayloadType == handler.SourcePayloadType() &&
				route.TargetPayloadType == handler.TargetPayloadType() {
				route.TranslationHandler = handler
				break
			}
		}
		if route.TranslationHandler == nil {
			logg.Fatal("route specification %q is invalid: do not know how to translate from %s to %s",
				routeSpec, route.SourcePayloadType, route.TargetPayloadType)
		}
		err = route.TranslationHandler.Init()
		if err != nil {
			logg.Fatal("while parsing route specification %q: cannot initialize translation from %s to %s: %s",
				routeSpec, route.SourcePayloadType, route.TargetPayloadType, err.Error())
		}

		//select delivery handler
		for _, handler := range allDeliveryHandlers {
			if route.TargetPayloadType == handler.PayloadType() {
				route.DeliveryHandler = handler
				break
			}
		}
		if route.DeliveryHandler == nil {
			logg.Fatal("route specification %q is invalid: cannot deliver %s",
				routeSpec, route.TargetPayloadType)
		}
		err = route.DeliveryHandler.Init()
		if err != nil {
			logg.Fatal("while parsing route specification %q: cannot initialize delivery for %s: %s",
				routeSpec, route.TargetPayloadType, err.Error())
		}

		cfg.EnabledRoutes = append(cfg.EnabledRoutes, route)
	}
	if len(cfg.EnabledRoutes) == 0 {
		logg.Fatal("missing required environment variable: TENSO_ROUTES")
	}

	return cfg
}

//GetenvOrDefault is like os.Getenv but it also takes a default value which is
//returned if the given environment variable is missing or empty.
func GetenvOrDefault(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		val = defaultVal
	}
	return val
}

func getDBURL() url.URL {
	dbName := GetenvOrDefault("TENSO_DB_NAME", "tenso")
	dbUsername := GetenvOrDefault("TENSO_DB_USERNAME", "postgres")
	dbPass := os.Getenv("TENSO_DB_PASSWORD")
	dbHost := GetenvOrDefault("TENSO_DB_HOSTNAME", "localhost")
	dbPort := GetenvOrDefault("TENSO_DB_PORT", "5432")

	dbConnOpts, err := url.ParseQuery(os.Getenv("TENSO_DB_CONNECTION_OPTIONS"))
	if err != nil {
		logg.Fatal("while parsing TENSO_DB_CONNECTION_OPTIONS: " + err.Error())
	}
	hostname, err := os.Hostname()
	if err == nil {
		dbConnOpts.Set("application_name", fmt.Sprintf("%s@%s", Component, hostname))
	} else {
		dbConnOpts.Set("application_name", Component)
	}

	return url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(dbUsername, dbPass),
		Host:     net.JoinHostPort(dbHost, dbPort),
		Path:     dbName,
		RawQuery: dbConnOpts.Encode(),
	}
}
