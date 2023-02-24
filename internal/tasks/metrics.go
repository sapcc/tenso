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

package tasks

import "github.com/prometheus/client_golang/prometheus"

var (
	eventConversionFailedCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tenso_failed_event_conversions",
			Help: "Counter for failed conversions of event payloads.",
		},
		[]string{"source_payload_type", "target_payload_type"},
	)
	eventConversionSuccessCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tenso_successful_event_conversions",
			Help: "Counter for successful conversions of event payloads.",
		},
		[]string{"source_payload_type", "target_payload_type"},
	)
	eventDeliveryFailedCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tenso_failed_event_deliveries",
			Help: "Counter for failed deliveries of event payloads.",
		},
		[]string{"payload_type"},
	)
	eventDeliverySuccessCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tenso_successful_event_deliveries",
			Help: "Counter for successful deliveries of event payloads.",
		},
		[]string{"payload_type"},
	)
	gcFailedCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "tenso_failed_garbage_collections",
			Help: "Counter for failed database GC runs.",
		},
	)
	gcSuccessCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "tenso_successful_garbage_collections",
			Help: "Counter for successful database GC runs.",
		},
	)
)

func init() {
	prometheus.MustRegister(eventConversionFailedCounter)
	prometheus.MustRegister(eventConversionSuccessCounter)
	prometheus.MustRegister(eventDeliveryFailedCounter)
	prometheus.MustRegister(eventDeliverySuccessCounter)
	prometheus.MustRegister(gcFailedCounter)
	prometheus.MustRegister(gcSuccessCounter)
}
