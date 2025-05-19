// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package tenso

import (
	"context"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/majewsky/schwift/v2"
	"github.com/majewsky/schwift/v2/gopherschwift"
	"github.com/sapcc/go-bits/osext"
)

// InitializeSwiftDelivery provides the shared Init() behavior for DeliveryHandler
// implementations that deliver to Swift. The target container name must be
// provided by the user in the environment variable with the given name.
func InitializeSwiftDelivery(ctx context.Context, pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts, envVarName string) (*schwift.Container, error) {
	containerName, err := osext.NeedGetenv(envVarName)
	if err != nil {
		return nil, err
	}
	client, err := openstack.NewObjectStorageV1(pc, eo)
	if err != nil {
		return nil, err
	}
	swiftAccount, err := gopherschwift.Wrap(client, nil)
	if err != nil {
		return nil, err
	}
	return swiftAccount.Container(containerName).EnsureExists(ctx)
}
