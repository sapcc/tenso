// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package servicenow

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sapcc/go-bits/osext"
	"github.com/sapcc/go-bits/regexpext"
)

// MappingConfiguration is the structure of the config file at
// $TENSO_SERVICENOW_MAPPING_CONFIG_PATH.
type MappingConfiguration struct {
	// endpoints
	Endpoints ClientSet `json:"endpoints"`

	// rulesets per event type
	HelmDeployment            MappingRuleset `json:"helm-deployment"`
	ActiveDirectoryDeployment MappingRuleset `json:"active-directory-deployment"`
	AWXWorkflow               MappingRuleset `json:"awx-workflow"`
	TerraformDeployment       MappingRuleset `json:"terraform-deployment"`

	// datacenter mapping
	Regions           map[string][]string `json:"regions"`
	AvailabilityZones map[string]struct {
		Datacenters []string `json:"datacenters"`
		Environment string   `json:"environment"`
	} `json:"availability_zones"`
}

var mappingConfigAtPath = map[string]MappingConfiguration{}

// LoadMappingConfiguration loads the mapping configuration from the file specified in the given environment variable.
func LoadMappingConfiguration(envVarName string) (MappingConfiguration, error) {
	filePath, err := osext.NeedGetenv(envVarName)
	if err != nil {
		return MappingConfiguration{}, err
	}

	// reuse cached result if possible
	if _, ok := mappingConfigAtPath[filePath]; ok {
		return mappingConfigAtPath[filePath], nil
	}

	buf, err := os.ReadFile(filePath)
	if err != nil {
		return MappingConfiguration{}, err
	}

	var result MappingConfiguration
	err = json.Unmarshal(buf, &result)
	if err != nil {
		return MappingConfiguration{}, fmt.Errorf("while parsing %s: %w", filePath, err)
	}

	err = result.Endpoints.Init()
	if err != nil {
		return MappingConfiguration{}, fmt.Errorf("while parsing %s: %w", filePath, err)
	}

	mappingConfigAtPath[filePath] = result
	return result, nil
}

// MappingRuleset is a set of rules for filling missing fields in a Change object.
type MappingRuleset []MappingRule

// Evaluate returns the sum of all rules in this ruleset that match the given Change object.
// For each field, the last matching rule takes precedence.
func (rs MappingRuleset) Evaluate(chg Change, routingInfo map[string]string) (result MappingRule) {
	for _, r := range rs {
		if !r.matches(chg, routingInfo) {
			continue
		}
		if r.ChangeTemplateID != "" {
			result.ChangeTemplateID = r.ChangeTemplateID
		}
		if r.Assignee != "" {
			result.Assignee = r.Assignee
		}
		if r.ResponsibleManager != "" {
			result.ResponsibleManager = r.ResponsibleManager
		}
		if r.ServiceOffering != "" {
			result.ServiceOffering = r.ServiceOffering
		}
		if r.Requester != "" {
			result.Requester = r.Requester
		}
	}
	return result
}

// MappingRule is a rule for filling missing fields in a Change object.
type MappingRule struct {
	MatchSummary          regexpext.BoundedRegexp `json:"match_summary"`
	MatchServiceNowTarget string                  `json:"match_servicenow_target"`
	ChangeTemplateID      string                  `json:"change_template_id"`
	Assignee              string                  `json:"assignee"`
	ResponsibleManager    string                  `json:"responsible_manager"`
	ServiceOffering       string                  `json:"service_offering"`
	Requester             string                  `json:"requester"`
}

func (r MappingRule) matches(chg Change, routingInfo map[string]string) bool {
	if (r.MatchServiceNowTarget != "" && r.MatchServiceNowTarget != routingInfo["servicenow-target"]) ||
		r.MatchSummary != "" && !r.MatchSummary.MatchString(chg.Summary) {
		return false
	}
	return true
}
