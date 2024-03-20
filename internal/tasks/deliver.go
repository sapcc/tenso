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

import (
	"context"
	"fmt"
	"time"

	"github.com/go-gorp/gorp/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sapcc/go-bits/jobloop"
	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/go-bits/sqlext"

	"github.com/sapcc/tenso/internal/tenso"
)

// WARNING: This must be run in a transaction, or else `FOR UPDATE SKIP LOCKED`
// will not work as expected.
var selectNextDeliveryQuery = sqlext.SimplifyWhitespace(`
	SELECT * FROM pending_deliveries
	 WHERE converted_at IS NOT NULL AND next_delivery_at <= $1
	 ORDER BY next_delivery_at ASC, payload_type ASC   -- secondary order ensures deterministic behavior during test
	 LIMIT 1 FOR UPDATE SKIP LOCKED
`)

const (
	DeliveryRetryInterval = 2 * time.Minute
)

func (c *Context) DeliveryJob(registerer prometheus.Registerer) jobloop.Job {
	return (&jobloop.TxGuardedJob[*gorp.Transaction, tenso.PendingDelivery]{
		Metadata: jobloop.JobMetadata{
			ReadableName:    "Event delivery",
			ConcurrencySafe: true, // because "FOR UPDATE SKIP LOCKED" is used
			CounterOpts: prometheus.CounterOpts{
				Name: "tenso_event_deliveries",
				Help: "Counter for deliveries of event payloads.",
			},
			CounterLabels: []string{"payload_type"},
		},
		BeginTx: c.DB.Begin,
		DiscoverRow: func(_ context.Context, tx *gorp.Transaction, _ prometheus.Labels) (pd tenso.PendingDelivery, err error) {
			err = tx.SelectOne(&pd, selectNextDeliveryQuery, c.timeNow())
			return pd, err
		},
		ProcessRow: c.processDelivery,
	}).Setup(registerer)
}

func (c *Context) processDelivery(ctx context.Context, tx *gorp.Transaction, pd tenso.PendingDelivery, labels prometheus.Labels) (returnedError error) {
	var event tenso.Event

	labels["payload_type"] = pd.PayloadType

	defer func() {
		if returnedError == nil {
			logg.Info("delivered %s payload for event %d (%q)", pd.PayloadType, pd.EventID, event.Description)
		} else {
			returnedError = fmt.Errorf("while trying to deliver %s payload for event %d (%q): %w", pd.PayloadType, pd.EventID, event.Description, returnedError)
		}
	}()

	// find the corresponding event
	err := tx.SelectOne(&event, `SELECT * FROM events WHERE id = $1`, pd.EventID)
	if err != nil {
		return err
	}

	// find the delivery handler
	var dh tenso.DeliveryHandler
	for _, route := range c.Config.EnabledRoutes {
		if route.TargetPayloadType == pd.PayloadType {
			dh = route.DeliveryHandler
			break
		}
	}
	if dh == nil {
		return fmt.Errorf("no DeliveryHandler found for %s (was this route disabled recently?)", pd.PayloadType)
	}

	// try to translate the payload, or set up a delayed retry on failure
	dlog, err := dh.DeliverPayload(ctx, []byte(*pd.Payload))
	if err != nil {
		pd.NextDeliveryAt = c.timeNow().Add(DeliveryRetryInterval)
		pd.FailedDeliveryCount++
		_, err2 := tx.Update(&pd)
		if err2 == nil {
			err2 = tx.Commit()
		}
		if err2 != nil {
			return fmt.Errorf("delivery failed: %w (additional error during DB update: %s)", err, err2.Error())
		}
		return fmt.Errorf("delivery failed: %w", err)
	}
	if dlog != nil {
		logg.Info("delivery of %s payload for event %d (%q) reported: %s", pd.PayloadType, pd.EventID, event.Description, dlog.Message)
	}
	logg.Debug("delivered %s payload for event %d (%q) was: %s", pd.PayloadType, pd.EventID, event.Description, *pd.Payload)

	// on successful delivery, remove the PendingDelivery
	_, err = tx.Delete(&pd)
	if err != nil {
		return err
	}
	return tx.Commit()
}
