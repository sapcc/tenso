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

package handlers

import (
	"errors"

	"github.com/gophercloud/gophercloud"
	"github.com/sapcc/tenso/internal/tenso"
)

func init() {
	tenso.ValidationHandlerRegistry.Add(func() tenso.ValidationHandler { return &terraformDeploymentValidator{} })
	tenso.TranslationHandlerRegistry.Add(func() tenso.TranslationHandler { return &terraformDeploymentToSNowTranslator{} })
	tenso.DeliveryHandlerRegistry.Add(func() tenso.DeliveryHandler { return &terraformDeploymentToSNowDeliverer{} })
}

////////////////////////////////////////////////////////////////////////////////
// ValidationHandler

type terraformDeploymentValidator struct {
}

func (v *terraformDeploymentValidator) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}

func (v *terraformDeploymentValidator) PluginTypeID() string {
	return "terraform-deployment-from-concourse.v1"
}

func (v *terraformDeploymentValidator) ValidatePayload(payload []byte) (*tenso.PayloadInfo, error) {
	//TODO: For now, this is only deployed to QA, and we allow everything because
	//we are working on the event source implementation first. Once that is done,
	//we will get rid of the events posted thus far, and add validation,
	//translation and delivery here.
	return &tenso.PayloadInfo{}, nil
}

////////////////////////////////////////////////////////////////////////////////
// TranslationHandler for SNow

type terraformDeploymentToSNowTranslator struct {
}

func (t *terraformDeploymentToSNowTranslator) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}

func (t *terraformDeploymentToSNowTranslator) PluginTypeID() string {
	return "terraform-deployment-from-concourse.v1->terraform-deployment-to-servicenow.v1"
}

func (t *terraformDeploymentToSNowTranslator) TranslatePayload(payload []byte) ([]byte, error) {
	//TODO: stub
	return nil, errors.New("unimplemented")
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for SNow

type terraformDeploymentToSNowDeliverer struct {
}

func (d *terraformDeploymentToSNowDeliverer) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}

func (d *terraformDeploymentToSNowDeliverer) PluginTypeID() string {
	return "terraform-deployment-from-concourse.v1->terraform-deployment-to-servicenow.v1"
}

func (d *terraformDeploymentToSNowDeliverer) DeliverPayload(payload []byte) (*tenso.DeliveryLog, error) {
	//TODO: stub
	return nil, errors.New("unimplemented")
}
