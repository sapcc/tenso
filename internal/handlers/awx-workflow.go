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

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/majewsky/schwift"
	"github.com/majewsky/schwift/gopherschwift"
	"github.com/sapcc/go-api-declarations/deployevent"
	"github.com/sapcc/go-bits/osext"

	"github.com/sapcc/tenso/internal/servicenow"
	"github.com/sapcc/tenso/internal/tenso"
)

func init() {
	tenso.ValidationHandlerRegistry.Add(func() tenso.ValidationHandler { return &awxWorkflowValidator{} })
	tenso.DeliveryHandlerRegistry.Add(func() tenso.DeliveryHandler { return &awxWorkflowToSwiftDeliverer{} })
	tenso.TranslationHandlerRegistry.Add(func() tenso.TranslationHandler { return &awxWorkflowToSNowTranslator{} })
	tenso.DeliveryHandlerRegistry.Add(func() tenso.DeliveryHandler { return &awxWorkflowToSNowDeliverer{} })
}

////////////////////////////////////////////////////////////////////////////////
// shared types

type awxWorkflowEvent struct {
	ID               uint             `json:"id"`
	Name             string           `json:"name"`
	URL              string           `json:"url"`
	CreatedBy        string           `json:"created_by"` //NOTE: not always a well-formed user ID, can also be something like "admin"
	StartedAt        *awxWorkflowTime `json:"started"`
	FinishedAt       *awxWorkflowTime `json:"finished"`
	Status           string           `json:"status"`
	Traceback        string           `json:"traceback"`
	Body             string           `json:"body"`
	AvailabilityZone string           `json:"inventory"`
	SearchQuery      string           `json:"limit"`
	ExtraVarsJSON    string           `json:"extra_vars"`
}

func (e awxWorkflowEvent) GetSummary() string {
	fields := []string{e.Name, e.AvailabilityZone}
	if e.SearchQuery != "" {
		fields = append(fields, e.SearchQuery)
	}
	return strings.Join(fields, ", ")
}

func (e awxWorkflowEvent) GetDescription() string {
	queryLine := "Inventory: " + e.AvailabilityZone
	if e.SearchQuery != "" {
		queryLine += " Limit: " + e.SearchQuery
	}
	lines := []string{
		fmt.Sprintf("Workflow %q started by %s finished %s", e.Name, strings.ToUpper(e.CreatedBy), e.Status),
		queryLine,
		"Link: " + e.URL,
	}
	if e.ExtraVarsJSON != "" {
		lines = append(lines, e.ExtraVarsJSON)
	}
	lines = append(lines, e.Body)
	return strings.Join(lines, "\n")
}

type awxWorkflowTime struct {
	time.Time
}

func (t *awxWorkflowTime) UnmarshalJSON(buf []byte) error {
	var in string
	err := json.Unmarshal(buf, &in)
	if err != nil {
		return err
	}
	t.Time, err = time.Parse("2006-01-02T15:04:05.999999-07:00", in)
	return err
}

////////////////////////////////////////////////////////////////////////////////
// ValidationHandler

type awxWorkflowValidator struct {
}

func (a *awxWorkflowValidator) Init(*gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}

func (a *awxWorkflowValidator) PluginTypeID() string {
	return "infra-workflow-from-awx.v1"
}

var (
	availabilityZoneRx = regexp.MustCompile(`^[a-z]{2}-[a-z]{2}-[0-9][a-z]$`) // e.g. "qa-de-1a"
)

func (a *awxWorkflowValidator) ValidatePayload(payload []byte) (*tenso.PayloadInfo, error) {
	event, err := jsonUnmarshalStrict[awxWorkflowEvent](payload)
	if err != nil {
		return nil, err
	}

	if event.ID == 0 {
		return nil, errors.New(`missing value for field "id"`)
	}
	if event.Name == "" {
		return nil, errors.New(`missing value for field "name"`)
	}
	if event.StartedAt == nil {
		return nil, errors.New(`missing value for field "started"`)
	}
	if event.FinishedAt == nil {
		return nil, errors.New(`missing value for field "finished"`)
	}
	if _, ok := awxOutcomes[event.Status]; !ok {
		return nil, fmt.Errorf(`invalid value for field "status": %q`, event.Status)
	}
	if !availabilityZoneRx.MatchString(event.AvailabilityZone) {
		return nil, fmt.Errorf(`invalid value for field "inventory": %q is not an AZ name`, event.AvailabilityZone)
	}

	return &tenso.PayloadInfo{Description: event.GetSummary()}, nil
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for Swift

type awxWorkflowToSwiftDeliverer struct {
	Container *schwift.Container
}

func (a *awxWorkflowToSwiftDeliverer) Init(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) error {
	containerName, err := osext.NeedGetenv("TENSO_AWX_WORKFLOW_SWIFT_CONTAINER")
	if err != nil {
		return err
	}

	client, err := openstack.NewObjectStorageV1(pc, eo)
	if err != nil {
		return err
	}

	swiftAccount, err := gopherschwift.Wrap(client, nil)
	if err != nil {
		return err
	}

	a.Container, err = swiftAccount.Container(containerName).EnsureExists()
	return err
}

func (a *awxWorkflowToSwiftDeliverer) PluginTypeID() string {
	return "infra-workflow-to-swift.v1"
}

func (a *awxWorkflowToSwiftDeliverer) DeliverPayload(_ context.Context, payload []byte, routingInfo map[string]string) (*tenso.DeliveryLog, error) {
	event, err := jsonUnmarshalStrict[awxWorkflowEvent](payload)
	if err != nil {
		return nil, err
	}

	objectName := fmt.Sprintf("%s/%d/%s.json",
		event.Name, event.ID, event.FinishedAt.Format(time.RFC3339),
	)
	return nil, a.Container.Object(objectName).Upload(bytes.NewReader(payload), nil, nil)
}

////////////////////////////////////////////////////////////////////////////////
// TranslationHandler for SNow

type awxWorkflowToSNowTranslator struct {
	Mapping servicenow.MappingConfiguration
}

var awxOutcomes = map[string]deployevent.Outcome{
	"successful": deployevent.OutcomeSucceeded,
}

func (a *awxWorkflowToSNowTranslator) Init(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) (err error) {
	a.Mapping, err = servicenow.LoadMappingConfiguration("TENSO_SERVICENOW_MAPPING_CONFIG_PATH")
	return err
}

func (a *awxWorkflowToSNowTranslator) PluginTypeID() string {
	return "infra-workflow-from-awx.v1->infra-workflow-to-servicenow.v1"
}

var (
	// If `event.SearchQuery` exactly matches this query, we will give it to SNow
	// in the "configuration_item" field. (We have to be strict here since
	// "configuration_item" is a reference to objects that exist in the SNow DB,
	// so we need to be reasonably sure that SNow knows about the object in
	// question.)
	configurationItemRx = regexp.MustCompile(`^node\d{3}-bb\d{3}\.cc\.[a-z]{2}-[a-z]{2}-[0-9]\.cloud\.sap$`) // e.g. "node002-bb091.cc.qa-de-1.cloud.sap"
)

func (a *awxWorkflowToSNowTranslator) TranslatePayload(payload []byte, routingInfo map[string]string) ([]byte, error) {
	event, err := jsonUnmarshalStrict[awxWorkflowEvent](payload)
	if err != nil {
		return nil, err
	}

	//TODO: fill "configuration_item" from event.SearchQuery, ONLY if the value is a valid node name or if we can extract a building block ID from it
	chg := servicenow.Change{
		StartedAt:        &event.StartedAt.Time,
		EndedAt:          &event.FinishedAt.Time,
		Outcome:          awxOutcomes[event.Status],
		Summary:          event.GetSummary(),
		Description:      event.GetDescription(),
		AvailabilityZone: event.AvailabilityZone,
		Executee:         strings.ToUpper(event.CreatedBy),
	}
	if configurationItemRx.MatchString(event.SearchQuery) {
		chg.ConfigurationItem = event.SearchQuery
	}
	if !sapUserIDRx.MatchString(chg.Executee) {
		chg.Executee = ""
	}

	return chg.Serialize(a.Mapping, a.Mapping.AWXWorkflow)
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for SNow

type awxWorkflowToSNowDeliverer struct {
	Mapping servicenow.MappingConfiguration
}

func (a *awxWorkflowToSNowDeliverer) Init(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) (err error) {
	a.Mapping, err = servicenow.LoadMappingConfiguration("TENSO_SERVICENOW_MAPPING_CONFIG_PATH")
	return err
}

func (a *awxWorkflowToSNowDeliverer) PluginTypeID() string {
	return "infra-workflow-to-servicenow.v1"
}

func (a *awxWorkflowToSNowDeliverer) DeliverPayload(ctx context.Context, payload []byte, routingInfo map[string]string) (*tenso.DeliveryLog, error) {
	// if the TranslationHandler wants us to ignore this payload, skip the delivery
	if string(payload) == "skip" {
		return nil, nil
	}
	return a.Mapping.Endpoints.Default.DeliverChangePayload(ctx, payload)
}
