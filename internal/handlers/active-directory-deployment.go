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

//nolint:dupl
package handlers

import (
	"errors"

	"github.com/gophercloud/gophercloud"

	"github.com/sapcc/tenso/internal/tenso"
)

func init() {
	tenso.ValidationHandlerRegistry.Add(func() tenso.ValidationHandler { return &activeDirectoryDeploymentValidator{} })
	tenso.TranslationHandlerRegistry.Add(func() tenso.TranslationHandler { return &activeDirectoryDeploymentToSNowTranslator{} })
	tenso.DeliveryHandlerRegistry.Add(func() tenso.DeliveryHandler { return &activeDirectoryDeploymentToSNowDeliverer{} })
}

////////////////////////////////////////////////////////////////////////////////
// ValidationHandler

type activeDirectoryDeploymentValidator struct {
}

func (v *activeDirectoryDeploymentValidator) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}

func (v *activeDirectoryDeploymentValidator) PluginTypeID() string {
	return "active-directory-deployment-from-concourse.v1"
}

func (v *activeDirectoryDeploymentValidator) ValidatePayload(payload []byte) (*tenso.PayloadInfo, error) {
	//TODO: For now, this is only deployed to QA, and we allow everything because
	//we are working on the event source implementation first. Once that is done,
	//we will get rid of the events posted thus far, and add validation,
	//translation and delivery here.
	return &tenso.PayloadInfo{}, nil
}

////////////////////////////////////////////////////////////////////////////////
// TranslationHandler for SNow

type activeDirectoryDeploymentToSNowTranslator struct {
}

func (t *activeDirectoryDeploymentToSNowTranslator) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}

func (t *activeDirectoryDeploymentToSNowTranslator) PluginTypeID() string {
	return "active-directory-deployment-from-concourse.v1->active-directory-deployment-to-servicenow.v1"
}

func (t *activeDirectoryDeploymentToSNowTranslator) TranslatePayload(payload []byte) ([]byte, error) {
	//TODO: stub
	return nil, errors.New("unimplemented")
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for SNow

type activeDirectoryDeploymentToSNowDeliverer struct {
}

func (d *activeDirectoryDeploymentToSNowDeliverer) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}

func (d *activeDirectoryDeploymentToSNowDeliverer) PluginTypeID() string {
	return "active-directory-deployment-to-servicenow.v1"
}

func (d *activeDirectoryDeploymentToSNowDeliverer) DeliverPayload(payload []byte) (*tenso.DeliveryLog, error) {
	//TODO: stub
	return nil, errors.New("unimplemented")
}
