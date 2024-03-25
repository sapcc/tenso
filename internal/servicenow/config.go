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

package servicenow

import (
	"os"

	"github.com/sapcc/go-bits/osext"
	"github.com/sapcc/go-bits/regexpext"
	"gopkg.in/yaml.v2"
)

// MappingConfiguration is the structure of the config file at
// $TENSO_SERVICENOW_MAPPING_CONFIG_PATH.
type MappingConfiguration struct {
	// rulesets per event type
	HelmDeployment            MappingRuleset `yaml:"helm-deployment"`
	ActiveDirectoryDeployment MappingRuleset `yaml:"active-directory-deployment"`
	AWXWorkflow               MappingRuleset `yaml:"awx-workflow"`
	TerraformDeployment       MappingRuleset `yaml:"terraform-deployment"`
	// datacenter mapping
	Regions           map[string][]string `yaml:"regions"`
	AvailabilityZones map[string]struct {
		Datacenters []string `yaml:"datacenters"`
		Environment string   `yaml:"environment"`
	} `yaml:"availability_zones"`
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
	err = yaml.UnmarshalStrict(buf, &result)
	if err == nil {
		mappingConfigAtPath[filePath] = result
	}
	return result, err
}

// MappingRuleset is a set of rules for filling missing fields in a Change object.
type MappingRuleset []MappingRule

// Evaluate returns the sum of all rules in this ruleset that match the given Change object.
func (rs MappingRuleset) Evaluate(chg Change) (result MappingRule) {
	for _, r := range rs {
		if !r.matches(chg) {
			continue
		}
		if r.ChangeModel != "" {
			result.ChangeModel = r.ChangeModel
		}
		if r.Fallbacks.Assignee != "" {
			result.Fallbacks.Assignee = r.Fallbacks.Assignee
		}
		if r.Fallbacks.Requester != "" {
			result.Fallbacks.Requester = r.Fallbacks.Requester
		}
		if r.Fallbacks.ResponsibleManager != "" {
			result.Fallbacks.ResponsibleManager = r.Fallbacks.ResponsibleManager
		}
		if r.Fallbacks.ServiceOffering != "" {
			result.Fallbacks.ServiceOffering = r.Fallbacks.ServiceOffering
		}
		if r.Overrides.Assignee != "" {
			result.Overrides.Assignee = r.Overrides.Assignee
		}
	}
	return result
}

// MappingRule is a rule for filling missing fields in a Change object.
type MappingRule struct {
	MatchEnvVars map[string]regexpext.BoundedRegexp `yaml:"match_env_vars"`
	MatchSummary regexpext.BoundedRegexp            `yaml:"match_summary"`
	ChangeModel  string                             `yaml:"change_model"`
	Fallbacks    struct {
		Assignee           string `yaml:"assignee"`
		Requester          string `yaml:"requester"`
		ResponsibleManager string `yaml:"responsible_manager"`
		ServiceOffering    string `yaml:"service_offering"`
	} `yaml:"fallbacks"`
	Overrides struct {
		Assignee string `yaml:"assignee"`
	} `yaml:"overrides"`
}

func (r MappingRule) matches(chg Change) bool {
	for key, rx := range r.MatchEnvVars {
		if !rx.MatchString(os.Getenv(key)) {
			return false
		}
	}
	if r.MatchSummary != "" && !r.MatchSummary.MatchString(chg.Summary) {
		return false
	}
	return true
}

// Assignee chooses the appropriate value for "assigned_to". The given value
// may be overridden, or a fallback may be applied if no value is given.
func (r MappingRule) Assignee(value string) string {
	if r.Overrides.Assignee != "" {
		return r.Overrides.Assignee
	}
	if value == "" {
		return r.Fallbacks.Assignee
	}
	return value
}

// Requester chooses the appropriate value for "requested_by". A fallback may
// be applied if no value is given.
func (r MappingRule) Requester(value string) string {
	if value == "" {
		return r.Fallbacks.Requester
	}
	return value
}

// ServiceOffering chooses the appropriate value for "service_offering".
func (r MappingRule) ServiceOffering() string {
	return r.Fallbacks.ServiceOffering
}

// ResponsibleManager chooses the appropriate value for "u_responsible_manager".
func (r MappingRule) ResponsibleManager() string {
	return r.Fallbacks.ResponsibleManager
}
