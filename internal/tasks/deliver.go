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
	"database/sql"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sapcc/go-bits/logg"
	"gopkg.in/gorp.v2"

	"github.com/sapcc/tenso/internal/tenso"
)

//WARNING: This must be run in a transaction, or else `FOR UPDATE SKIP LOCKED`
//will not work as expected.
var selectNextDeliveryQuery = tenso.SimplifyWhitespaceInSQL(`
	SELECT * FROM pending_deliveries
	 WHERE converted_at IS NOT NULL AND next_delivery_at <= $1
	 ORDER BY next_delivery_at ASC, payload_type ASC   -- secondary order ensures deterministic behavior during test
	 LIMIT 1 FOR UPDATE SKIP LOCKED
`)

const (
	DeliveryRetryInterval = 2 * time.Minute
)

//PollForPendingConversions is a JobPoller that finds the next pending
//conversion job. The returned Job tries to execute the conversion.
func (c *Context) PollForPendingDeliveries() (j Job, returnedError error) {
	defer func() {
		if returnedError != nil && returnedError != sql.ErrNoRows {
			labels := prometheus.Labels{"payload_type": "early-db-access"}
			eventDeliveryFailedCounter.With(labels).Inc()
		}
	}()

	//we need a DB transaction for the row-level locking to work correctly
	tx, err := c.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		if returnedError != nil {
			tenso.RollbackUnlessCommitted(tx)
		}
	}()

	//select the next PendingDelivery without converted payload
	var pd tenso.PendingDelivery
	err = tx.SelectOne(&pd, selectNextDeliveryQuery, c.timeNow())
	if err != nil {
		if err == sql.ErrNoRows {
			logg.Debug("no payloads to deliver - slowing down...")
			//nolint:errcheck
			tx.Rollback() //avoid the log line generated by core.RollbackUnlessCommitted()
			return nil, sql.ErrNoRows
		}
		return nil, err
	}

	return payloadDeliverJob{c, tx, pd}, nil
}

type payloadDeliverJob struct {
	c  *Context
	tx *gorp.Transaction
	pd tenso.PendingDelivery
}

//Execute implements the Job interface.
func (j payloadDeliverJob) Execute() (returnedError error) {
	c, tx, pd := j.c, j.tx, j.pd

	defer tenso.RollbackUnlessCommitted(tx)

	var event tenso.Event

	defer func() {
		labels := prometheus.Labels{"payload_type": pd.PayloadType}
		if returnedError == nil {
			eventDeliverySuccessCounter.With(labels).Inc()
			logg.Info("delivered %s payload for event %d (%q)", pd.PayloadType, pd.EventID, event.Description)
		} else {
			eventDeliveryFailedCounter.With(labels).Inc()
			returnedError = fmt.Errorf("while trying to deliver %s payload for event %d (%q): %w", pd.PayloadType, pd.EventID, event.Description, returnedError)
		}
	}()

	//find the corresponding event
	err := tx.SelectOne(&event, `SELECT * FROM events WHERE id = $1`, pd.EventID)
	if err != nil {
		return err
	}

	//when running in a unit test, wait for the test harness to unblock us
	if c.Blocker != nil {
		for range c.Blocker {
		}
	}

	//find the delivery handler
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

	//try to translate the payload, or set up a delayed retry on failure
	dlog, err := dh.DeliverPayload([]byte(*pd.Payload))
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

	//on successful delivery, remove the PendingDelivery
	_, err = tx.Delete(&pd)
	if err != nil {
		return err
	}
	return tx.Commit()
}
