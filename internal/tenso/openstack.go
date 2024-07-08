/*******************************************************************************
*
* Copyright 2023 SAP SE
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
