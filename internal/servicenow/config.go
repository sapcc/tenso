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
	"gopkg.in/yaml.v2"
)

// MappingConfiguration is the structure of the config file at
// $TENSO_SERVICENOW_MAPPING_CONFIG_PATH.
type MappingConfiguration struct {
	//rulesets per event type
	HelmDeployment            MappingRuleset `yaml:"helm-deployment"`
	ActiveDirectoryDeployment MappingRuleset `yaml:"active-directory-deployment"`
	AWXWorkflow               MappingRuleset `yaml:"awx-workflow"`
	TerraformDeployment       MappingRuleset `yaml:"terraform-deployment"`
	//datacenter mapping
	Regions           map[string][]string `yaml:"regions"`
	AvailabilityZones map[string]struct {
		Datacenters []string `yaml:"datacenters"`
		Environment string   `yaml:"environment"`
	} `yaml:"availability_zones"`
}

// LoadMappingConfiguration loads the mapping configuration from
// $TENSO_SERVICENOW_MAPPING_CONFIG_PATH.
func LoadMappingConfiguration() (MappingConfiguration, error) {
	filePath, err := osext.NeedGetenv("TENSO_SERVICENOW_MAPPING_CONFIG_PATH")
	if err != nil {
		return MappingConfiguration{}, err
	}

	buf, err := os.ReadFile(filePath)
	if err != nil {
		return MappingConfiguration{}, err
	}

	var result MappingConfiguration
	err = yaml.UnmarshalStrict(buf, &result)
	return result, err
}

// MappingRuleset is a set of rules for filling missing fields in a Change
// object.
type MappingRuleset struct {
	Fallbacks struct {
		Assignee           string `yaml:"assignee"`
		Requester          string `yaml:"requester"`
		ResponsibleManager string `yaml:"responsible_manager"`
		ServiceOffering    string `yaml:"service_offering"`
	} `yaml:"fallbacks"`
	Overrides struct {
		Assignee string `yaml:"assignee"`
	} `yaml:"overrides"`
}

// Assignee chooses the appropriate value for "assigned_to". The given value
// may be overridden, or a fallback may be applied if no value is given.
func (rs MappingRuleset) Assignee(value string) string {
	if rs.Overrides.Assignee != "" {
		return rs.Overrides.Assignee
	}
	if value == "" {
		return rs.Fallbacks.Assignee
	}
	return value
}

// Requester chooses the appropriate value for "requested_by". A fallback may
// be applied if no value is given.
func (rs MappingRuleset) Requester(value string) string {
	if value == "" {
		return rs.Fallbacks.Requester
	}
	return value
}

// ServiceOffering chooses the appropriate value for "service_offering".
func (rs MappingRuleset) ServiceOffering() string {
	return rs.Fallbacks.ServiceOffering
}

// ResponsibleManager chooses the appropriate value for "u_responsible_manager".
func (rs MappingRuleset) ResponsibleManager() string {
	return rs.Fallbacks.ResponsibleManager
}
