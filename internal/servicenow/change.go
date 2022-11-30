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
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Change describes the data that we can pass into a ServiceNow change object.
type Change struct {
	StartedAt         *time.Time
	EndedAt           *time.Time
	CloseCode         string
	Summary           string
	Description       string
	ConfigurationItem string

	// Executee (optional) is the user ID of the user who triggered/executed this change.
	Executee string
	// Region or AvailabilityZone describes the system that was targeted. Exactly one needs to be set.
	Region           string
	AvailabilityZone string
}

// Serialize returns the payload that we can send into SNow.
func (chg Change) Serialize(cfg MappingConfiguration, ruleset MappingRuleset) ([]byte, error) {
	//find AZs for this change
	if chg.Region == "" && chg.AvailabilityZone == "" {
		return nil, fmt.Errorf("cannot serialize a servicenow.Change without a value for either Region or AvailabilityZone")
	}
	var azNames []string
	switch {
	case chg.AvailabilityZone != "":
		azNames = []string{chg.AvailabilityZone}
	case chg.Region != "":
		var ok bool
		azNames, ok = cfg.Regions[chg.Region]
		if !ok {
			return nil, fmt.Errorf("region not found in mapping config: %q", chg.Region)
		}
	}

	//find datacenters and environment for this change from AZ mapping config
	var (
		datacenters []string
		environment string
	)
	for _, azName := range azNames {
		azMapping, ok := cfg.AvailabilityZones[azName]
		if !ok {
			return nil, fmt.Errorf("availability zone not found in mapping config: %q", azName)
		}
		datacenters = append(datacenters, azMapping.Datacenters...)
		if environment == "" {
			environment = azMapping.Environment
		} else if environment != azMapping.Environment {
			return nil, fmt.Errorf(`found inconsistent values of field "environment" across AZs of region %q`, chg.Region)
		}
	}

	data := map[string]interface{}{
		"chg_model":                "SAP Standard GCS Retrospective GCSCHGCCEEC147 CCloud Control Plane Deployment",
		"assigned_to":              ruleset.Assignee(chg.Executee),
		"requested_by":             ruleset.Requester(chg.Executee),
		"u_implementation_contact": chg.Executee,
		"service_offering":         ruleset.ServiceOffering(),
		"u_data_center":            strings.Join(datacenters, ", "),
		"u_customer_impact":        "No Impact",
		"u_responsible_manager":    ruleset.ResponsibleManager(),
		"u_customer_notification":  "No",
		"u_impacted_lobs":          "Global Cloud Services",
		"u_affected_environments":  environment,
		"start_date":               sNowTimeStr(chg.StartedAt),
		"end_date":                 sNowTimeStr(chg.EndedAt),
		"close_code":               chg.CloseCode,
		"close_notes":              chg.Description,
		"short_description":        chg.Summary,
	}
	if chg.ConfigurationItem != "" {
		data["cmdb_ci"] = chg.ConfigurationItem
	}
	return json.Marshal(data)
}

func sNowTimeStr(t *time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}
