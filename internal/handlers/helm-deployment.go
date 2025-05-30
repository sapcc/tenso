// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/majewsky/schwift/v2"
	"github.com/sapcc/go-api-declarations/deployevent"
	"github.com/sapcc/go-bits/osext"

	"github.com/sapcc/tenso/internal/servicenow"
	"github.com/sapcc/tenso/internal/tenso"
)

//nolint:dupl
func init() {
	tenso.ValidationHandlerRegistry.Add(func() tenso.ValidationHandler { return &helmDeploymentValidator{} })
	tenso.DeliveryHandlerRegistry.Add(func() tenso.DeliveryHandler { return &helmDeploymentToElkDeliverer{} })
	tenso.DeliveryHandlerRegistry.Add(func() tenso.DeliveryHandler { return &helmDeploymentToSwiftDeliverer{} })
	tenso.TranslationHandlerRegistry.Add(func() tenso.TranslationHandler { return &helmDeploymentToSNowTranslator{} })
	tenso.DeliveryHandlerRegistry.Add(func() tenso.DeliveryHandler { return &helmDeploymentToSNowDeliverer{} })
}

func releaseDescriptorsOf(event deployevent.Event, sep string) (result []string) {
	for _, hr := range event.HelmReleases {
		result = append(result, fmt.Sprintf("%s%s%s", hr.Name, sep, hr.Cluster))
	}
	return
}

////////////////////////////////////////////////////////////////////////////////
// ValidationHandler

type helmDeploymentValidator struct {
}

func (h *helmDeploymentValidator) Init(context.Context, *gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	return nil
}

func (h *helmDeploymentValidator) PluginTypeID() string {
	return "helm-deployment-from-concourse.v1"
}

func (h *helmDeploymentValidator) ValidatePayload(payload []byte) (*tenso.PayloadInfo, error) {
	event, err := parseAndValidateDeployEvent(payload)
	if err != nil {
		return nil, err
	}

	if event.ADDeployment != nil {
		return nil, errors.New("active-directory-deployment may not be set for Helm deployment events")
	}
	if len(event.TerraformRuns) != 0 {
		return nil, errors.New("terraform-runs[] may not be set for Helm deployment events")
	}
	if len(event.HelmReleases) == 0 {
		return nil, errors.New("helm-release[] may not be empty")
	}
	for idx, relInfo := range event.HelmReleases {
		if relInfo == nil {
			return nil, fmt.Errorf(`helm-release[%d] may not be nil`, idx)
		}
		//TODO: Can we do regex matches to validate the contents of Name, Namespace, ChartID, ChartPath?
		if relInfo.Name == "" {
			return nil, fmt.Errorf(`invalid value for field helm-release[].name: %q`, relInfo.Name)
		}
		if !relInfo.Outcome.IsKnownInputValue() {
			return nil, fmt.Errorf(`in helm-release %q: invalid value for field outcome: %q`, relInfo.Name, relInfo.Outcome)
		}
		if relInfo.ChartID == "" && relInfo.ChartPath == "" {
			return nil, fmt.Errorf(`in helm-release %q: chart-id and chart-path can not both be empty`, relInfo.Name)
		}
		if relInfo.ChartID != "" && relInfo.ChartPath != "" {
			return nil, fmt.Errorf(`in helm-release %q: chart-id and chart-path can not both be set`, relInfo.Name)
		}
		if !clusterRx.MatchString(relInfo.Cluster) {
			return nil, fmt.Errorf(`in helm-release %q: invalid value for field cluster: %q`, relInfo.Name, relInfo.Cluster)
		}
		if !isClusterLocatedInRegion(relInfo.Cluster, event.Region) {
			return nil, fmt.Errorf(`in helm-release %q: cluster %q is not in region %q`, relInfo.Name, relInfo.Cluster, event.Region)
		}
		if relInfo.Namespace == "" {
			return nil, fmt.Errorf(`in helm-release %q: invalid value for field namespace: %q`, relInfo.Name, relInfo.Namespace)
		}
		if relInfo.StartedAt == nil && relInfo.Outcome != deployevent.OutcomeNotDeployed {
			return nil, fmt.Errorf(`in helm-release %q: field started-at must be set for outcome %q`, relInfo.Name, relInfo.Outcome)
		}
		if relInfo.StartedAt != nil && relInfo.Outcome == deployevent.OutcomeNotDeployed {
			return nil, fmt.Errorf(`in helm-release %q: field started-at may not be set for outcome %q`, relInfo.Name, relInfo.Outcome)
		}
		if relInfo.FinishedAt == nil && (relInfo.Outcome != deployevent.OutcomeNotDeployed && relInfo.Outcome != deployevent.OutcomeHelmUpgradeFailed) {
			return nil, fmt.Errorf(`in helm-release %q: field finished-at must be set for outcome %q`, relInfo.Name, relInfo.Outcome)
		}
		if relInfo.FinishedAt != nil && (relInfo.Outcome == deployevent.OutcomeNotDeployed || relInfo.Outcome == deployevent.OutcomeHelmUpgradeFailed) {
			return nil, fmt.Errorf(`in helm-release %q: field finished-at may not be set for outcome %q`, relInfo.Name, relInfo.Outcome)
		}
	}

	return &tenso.PayloadInfo{
		Description: fmt.Sprintf("%s/%s: deploy %s",
			event.Pipeline.TeamName, event.Pipeline.PipelineName,
			strings.Join(releaseDescriptorsOf(event, " to "), " and "),
		),
	}, nil
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for ELK

// helmDeploymentToElkDeliverer is a tenso.DeliveryHandler.
type helmDeploymentToElkDeliverer struct {
	LogstashHost string
}

func (h *helmDeploymentToElkDeliverer) Init(context.Context, *gophercloud.ProviderClient, gophercloud.EndpointOpts) error {
	var err error
	h.LogstashHost, err = osext.NeedGetenv("TENSO_HELM_DEPLOYMENT_LOGSTASH_HOST")
	if err != nil {
		return err
	}
	_, _, err = net.SplitHostPort(h.LogstashHost)
	if err != nil {
		return fmt.Errorf(`expected TENSO_HELM_DEPLOYMENT_LOGSTASH_HOST to look like "host:port", but got %q`,
			h.LogstashHost)
	}
	return nil
}

func (h *helmDeploymentToElkDeliverer) PluginTypeID() string {
	return "helm-deployment-to-elk.v1"
}

func (h *helmDeploymentToElkDeliverer) DeliverPayload(_ context.Context, payload []byte, routingInfo map[string]string) (*tenso.DeliveryLog, error) {
	// Logstash wants everything on one line, so ensure we don't have unnecessary whitespace in the payload
	var buf bytes.Buffer
	err := json.Compact(&buf, payload)
	if err != nil {
		return nil, err
	}
	err = buf.WriteByte('\n')
	if err != nil {
		return nil, err
	}
	payload = buf.Bytes()

	// deliver payload to Logstash
	conn, err := net.Dial("tcp", h.LogstashHost)
	if err != nil {
		return nil, err
	}
	_, err = conn.Write(payload)
	if err != nil {
		return nil, err
	}
	return nil, conn.Close()
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for Swift

type helmDeploymentToSwiftDeliverer struct {
	Container *schwift.Container
}

func (h *helmDeploymentToSwiftDeliverer) Init(ctx context.Context, pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) (err error) {
	h.Container, err = tenso.InitializeSwiftDelivery(ctx, pc, eo, "TENSO_HELM_DEPLOYMENT_SWIFT_CONTAINER")
	return err
}

func (h *helmDeploymentToSwiftDeliverer) PluginTypeID() string {
	return "helm-deployment-to-swift.v1"
}

func (h *helmDeploymentToSwiftDeliverer) DeliverPayload(ctx context.Context, payload []byte, routingInfo map[string]string) (*tenso.DeliveryLog, error) {
	event, err := jsonUnmarshalStrict[deployevent.Event](payload)
	if err != nil {
		return nil, err
	}

	objectName := fmt.Sprintf("%s/%s/%s/%s/%s.json",
		event.Pipeline.TeamName, event.Pipeline.PipelineName,
		strings.Join(releaseDescriptorsOf(event, "_"), ","),
		string(event.CombinedOutcome()),
		event.RecordedAt.Format(time.RFC3339),
	)
	return nil, h.Container.Object(objectName).Upload(ctx, bytes.NewReader(payload), nil, nil)
}

////////////////////////////////////////////////////////////////////////////////
// TranslationHandler for SNow

type helmDeploymentToSNowTranslator struct {
	Mapping servicenow.MappingConfiguration
}

func (h *helmDeploymentToSNowTranslator) Init(ctx context.Context, pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) (err error) {
	h.Mapping, err = servicenow.LoadMappingConfiguration("TENSO_SERVICENOW_MAPPING_CONFIG_PATH")
	return err
}

func (h *helmDeploymentToSNowTranslator) PluginTypeID() string {
	return "helm-deployment-from-concourse.v1->helm-deployment-to-servicenow.v1"
}

func (h *helmDeploymentToSNowTranslator) TranslatePayload(payload []byte, routingInfo map[string]string) ([]byte, error) {
	event, err := jsonUnmarshalStrict[deployevent.Event](payload)
	if err != nil {
		return nil, err
	}

	releaseDesc := strings.Join(releaseDescriptorsOf(event, " to "), ", ")
	inputDesc := strings.Join(inputDescriptorsOf(event), ", ")
	chg := servicenow.Change{
		StartedAt:   event.CombinedStartDate(),
		EndedAt:     event.RecordedAt,
		Outcome:     event.CombinedOutcome(),
		Summary:     "Deploy " + releaseDesc,
		Description: fmt.Sprintf("Deployed %s with versions: %s\nDeployment log: %s\n\nOutcome: %s", releaseDesc, inputDesc, event.Pipeline.BuildURL, string(event.CombinedOutcome())),
		Executee:    event.Pipeline.CreatedBy, //NOTE: can be empty
		Region:      event.Region,
	}

	return chg.Serialize(h.Mapping, h.Mapping.HelmDeployment, routingInfo)
}

////////////////////////////////////////////////////////////////////////////////
// DeliveryHandler for SNow

type helmDeploymentToSNowDeliverer struct {
	Mapping servicenow.MappingConfiguration
}

func (h *helmDeploymentToSNowDeliverer) Init(ctx context.Context, pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) (err error) {
	h.Mapping, err = servicenow.LoadMappingConfiguration("TENSO_SERVICENOW_MAPPING_CONFIG_PATH")
	return err
}

func (h *helmDeploymentToSNowDeliverer) PluginTypeID() string {
	return "helm-deployment-to-servicenow.v1"
}

func (h *helmDeploymentToSNowDeliverer) DeliverPayload(ctx context.Context, payload []byte, routingInfo map[string]string) (*tenso.DeliveryLog, error) {
	return h.Mapping.Endpoints.DeliverChangePayload(ctx, payload, routingInfo)
}
