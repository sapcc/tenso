// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package tenso

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/sapcc/go-bits/gophercloudext"
	"github.com/sapcc/go-bits/must"
	"github.com/sapcc/go-bits/osext"
)

// Configuration contains all configuration values that we collect from the environment.
type Configuration struct {
	DatabaseURL   url.URL
	EnabledRoutes []Route
}

var (
	payloadTypePattern = `[a-zA-Z0-9.-]+`
	payloadTypeRx      = regexp.MustCompile(fmt.Sprintf(`^%s$`, payloadTypePattern))
	routeSpecRx        = regexp.MustCompile(fmt.Sprintf(`^(%[1]s)\s*->\s*(%[1]s)$`, payloadTypePattern))
)

// ParseConfiguration obtains a tenso.Configuration instance from the
// corresponding environment variables. Aborts on error.
func ParseConfiguration(ctx context.Context) (Configuration, *gophercloud.ProviderClient, gophercloud.EndpointOpts) {
	var cfg Configuration

	// initialize OpenStack connection
	provider, eo, err := gophercloudext.NewProviderClient(ctx, nil)
	must.Succeed(err)

	cfg.EnabledRoutes = must.Return(BuildRoutes(ctx, strings.Split(osext.MustGetenv("TENSO_ROUTES"), ","), provider, eo))
	return cfg, provider, eo
}

// BuildRoutes is used by ParseConfiguration to process the TENSO_ROUTES env
// variable. It is an exported function to make it accessible in unit tests.
//
// The `pc` and `eo` args are passed to the handlers' Init() methods verbatim.
func BuildRoutes(ctx context.Context, routeSpecs []string, pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) ([]Route, error) {
	var (
		result              []Route
		validationHandlers  = make(map[string]ValidationHandler)
		translationHandlers = make(map[string]TranslationHandler)
		deliveryHandlers    = make(map[string]DeliveryHandler)
	)

	// parse routes
	for _, routeSpec := range routeSpecs {
		routeSpec = strings.TrimSpace(routeSpec)
		if routeSpec == "" {
			// be lenient e.g. when the list of routes has a trailing comma
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

		// instantiate validation handler if not done yet
		if validationHandlers[route.SourcePayloadType] == nil {
			vh := ValidationHandlerRegistry.Instantiate(route.SourcePayloadType)
			if vh == nil {
				return nil, fmt.Errorf("route specification %q is invalid: cannot validate %s",
					routeSpec, route.SourcePayloadType)
			}
			err := vh.Init(ctx, pc, eo)
			if err != nil {
				return nil, fmt.Errorf("while parsing route specification %q: cannot initialize validation for %s: %s",
					routeSpec, route.SourcePayloadType, err.Error())
			}
			validationHandlers[route.SourcePayloadType] = vh
		}
		route.ValidationHandler = validationHandlers[route.SourcePayloadType]

		// initiate translation handler if not done yet
		typeID := fmt.Sprintf("%s->%s", route.SourcePayloadType, route.TargetPayloadType)
		if translationHandlers[typeID] == nil {
			th := TranslationHandlerRegistry.Instantiate(typeID)
			if th == nil {
				return nil, fmt.Errorf("route specification %q is invalid: do not know how to translate from %s to %s",
					routeSpec, route.SourcePayloadType, route.TargetPayloadType)
			}
			err := th.Init(ctx, pc, eo)
			if err != nil {
				return nil, fmt.Errorf("while parsing route specification %q: cannot initialize translation from %s to %s: %s",
					routeSpec, route.SourcePayloadType, route.TargetPayloadType, err.Error())
			}
			translationHandlers[typeID] = th
		}
		route.TranslationHandler = translationHandlers[typeID]

		// instantiate delivery handler if not done yet
		if deliveryHandlers[route.TargetPayloadType] == nil {
			dh := DeliveryHandlerRegistry.Instantiate(route.TargetPayloadType)
			if dh == nil {
				return nil, fmt.Errorf("route specification %q is invalid: cannot deliver %s",
					routeSpec, route.TargetPayloadType)
			}
			err := dh.Init(ctx, pc, eo)
			if err != nil {
				return nil, fmt.Errorf("while parsing route specification %q: cannot initialize delivery for %s: %s",
					routeSpec, route.TargetPayloadType, err.Error())
			}
			deliveryHandlers[route.TargetPayloadType] = dh
		}
		route.DeliveryHandler = deliveryHandlers[route.TargetPayloadType]

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
