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
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/utils/openstack/clientconfig"
	"github.com/sapcc/go-bits/easypg"
	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/go-bits/must"
	"github.com/sapcc/go-bits/osext"
)

//Configuration contains all configuration values that we collect from the environment.
type Configuration struct {
	DatabaseURL   *url.URL
	EnabledRoutes []Route
}

var (
	payloadTypePattern = `[a-zA-Z0-9.-]+`
	payloadTypeRx      = regexp.MustCompile(fmt.Sprintf(`^%s$`, payloadTypePattern))
	routeSpecRx        = regexp.MustCompile(fmt.Sprintf(`^(%[1]s)\s*->\s*(%[1]s)$`, payloadTypePattern))
)

//ParseConfiguration obtains a tenso.Configuration instance from the
//corresponding environment variables. Aborts on error.
func ParseConfiguration() (Configuration, *gophercloud.ProviderClient, gophercloud.EndpointOpts) {
	var cfg Configuration
	cfg.DatabaseURL = must.Return(easypg.URLFrom(easypg.URLParts{
		HostName:          osext.GetenvOrDefault("TENSO_DB_HOSTNAME", "localhost"),
		Port:              osext.GetenvOrDefault("TENSO_DB_PORT", "5432"),
		UserName:          osext.GetenvOrDefault("TENSO_DB_USERNAME", "postgres"),
		Password:          os.Getenv("TENSO_DB_PASSWORD"),
		ConnectionOptions: os.Getenv("TENSO_DB_CONNECTION_OPTIONS"),
		DatabaseName:      osext.GetenvOrDefault("TENSO_DB_NAME", "tenso"),
	}))

	//initialize OpenStack connection
	ao, err := clientconfig.AuthOptions(nil)
	if err != nil {
		logg.Fatal("cannot find OpenStack credentials: " + err.Error())
	}
	ao.AllowReauth = true
	provider, err := openstack.AuthenticatedClient(*ao)
	if err != nil {
		logg.Fatal("cannot connect to OpenStack: " + err.Error())
	}
	eo := gophercloud.EndpointOpts{
		//note that empty values are acceptable in both fields
		Region:       os.Getenv("OS_REGION_NAME"),
		Availability: gophercloud.Availability(os.Getenv("OS_INTERFACE")),
	}

	cfg.EnabledRoutes = must.Return(BuildRoutes(strings.Split(osext.MustGetenv("TENSO_ROUTES"), ","), provider, eo))
	return cfg, provider, eo
}

//BuildRoutes is used by ParseConfiguration to process the TENSO_ROUTES env
//variable. It is an exported function to make it accessible in unit tests.
//
//The `pc` and `eo` args are passed to the handlers' Init() methods verbatim.
func BuildRoutes(routeSpecs []string, pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) ([]Route, error) {
	var result []Route

	//parse routes
	for _, routeSpec := range routeSpecs {
		routeSpec = strings.TrimSpace(routeSpec)
		if routeSpec == "" {
			//be lenient e.g. when the list of routes has a trailing comma
			continue
		}

		match := routeSpecRx.FindStringSubmatch(routeSpec)
		if match == nil {
			return nil, fmt.Errorf("route specification %q is invalid: syntax error", routeSpec)
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
			return nil, fmt.Errorf("route specification %q is invalid: cannot validate %s",
				routeSpec, route.SourcePayloadType)
		}
		err := route.ValidationHandler.Init(pc, eo)
		if err != nil {
			return nil, fmt.Errorf("while parsing route specification %q: cannot initialize validation for %s: %s",
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
			return nil, fmt.Errorf("route specification %q is invalid: do not know how to translate from %s to %s",
				routeSpec, route.SourcePayloadType, route.TargetPayloadType)
		}
		err = route.TranslationHandler.Init(pc, eo)
		if err != nil {
			return nil, fmt.Errorf("while parsing route specification %q: cannot initialize translation from %s to %s: %s",
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
			return nil, fmt.Errorf("route specification %q is invalid: cannot deliver %s",
				routeSpec, route.TargetPayloadType)
		}
		err = route.DeliveryHandler.Init(pc, eo)
		if err != nil {
			return nil, fmt.Errorf("while parsing route specification %q: cannot initialize delivery for %s: %s",
				routeSpec, route.TargetPayloadType, err.Error())
		}

		result = append(result, route)
	}

	if len(result) == 0 {
		return nil, errors.New("no routes specified")
	}

	return result, nil
}

func IsWellformedPayloadType(val string) bool {
	return payloadTypeRx.MatchString(val)
}
