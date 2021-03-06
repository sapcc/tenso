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
	"github.com/sapcc/go-bits/sqlext"
	"gopkg.in/gorp.v2"

	"github.com/sapcc/tenso/internal/tenso"
)

//WARNING: This must be run in a transaction, or else `FOR UPDATE SKIP LOCKED`
//will not work as expected.
var selectNextConversionQuery = sqlext.SimplifyWhitespace(`
	SELECT * FROM pending_deliveries
	 WHERE converted_at IS NULL AND next_conversion_at <= $1
	 ORDER BY next_conversion_at ASC, payload_type ASC   -- secondary order ensures deterministic behavior during test
	 LIMIT 1 FOR UPDATE SKIP LOCKED
`)

const (
	ConversionRetryInterval = 2 * time.Minute
)

//PollForPendingConversions is a JobPoller that finds the next pending
//conversion job. The returned Job tries to execute the conversion.
func (c *Context) PollForPendingConversions() (j Job, returnedError error) {
	defer func() {
		if returnedError != nil && returnedError != sql.ErrNoRows {
			labels := prometheus.Labels{"source_payload_type": "early-db-access", "target_payload_type": "early-db-access"}
			eventConversionFailedCounter.With(labels).Inc()
		}
	}()

	//we need a DB transaction for the row-level locking to work correctly
	tx, err := c.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		if returnedError != nil {
			sqlext.RollbackUnlessCommitted(tx)
		}
	}()

	//select the next PendingDelivery without converted payload
	var pd tenso.PendingDelivery
	err = tx.SelectOne(&pd, selectNextConversionQuery, c.timeNow())
	if err != nil {
		if err == sql.ErrNoRows {
			logg.Debug("no payloads to convert - slowing down...")
			//nolint:errcheck
			tx.Rollback() //avoid the log line generated by sqlext.RollbackUnlessCommitted()
			return nil, sql.ErrNoRows
		}
		return nil, err
	}

	return payloadConvertJob{c, tx, pd}, nil
}

type payloadConvertJob struct {
	c  *Context
	tx *gorp.Transaction
	pd tenso.PendingDelivery
}

//Execute implements the Job interface.
func (j payloadConvertJob) Execute() (returnedError error) {
	c, tx, pd := j.c, j.tx, j.pd

	defer sqlext.RollbackUnlessCommitted(tx)

	var event tenso.Event

	defer func() {
		labels := prometheus.Labels{"source_payload_type": event.PayloadType, "target_payload_type": pd.PayloadType}
		if event.PayloadType == "" { //because we did not get to loading it
			labels["source_payload_type"] = "early-db-access"
		}
		if returnedError == nil {
			eventConversionSuccessCounter.With(labels).Inc()
			logg.Info("converted payload for event %d (%q) into %s", pd.EventID, event.Description, pd.PayloadType)
		} else {
			eventConversionFailedCounter.With(labels).Inc()
			returnedError = fmt.Errorf("while trying to convert payload for event %d (%q) into %s: %w", pd.EventID, event.Description, pd.PayloadType, returnedError)
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

	//find the translation handler
	var th tenso.TranslationHandler
	for _, route := range c.Config.EnabledRoutes {
		if route.SourcePayloadType == event.PayloadType && route.TargetPayloadType == pd.PayloadType {
			th = route.TranslationHandler
			break
		}
	}
	if th == nil {
		return fmt.Errorf("no TranslationHandler found for %s -> %s (was this route disabled recently?)",
			event.PayloadType, pd.PayloadType)
	}

	//try to translate the payload, or set up a delayed retry on failure
	targetPayloadBytes, err := th.TranslatePayload([]byte(event.Payload))
	if err != nil {
		pd.NextConversionAt = c.timeNow().Add(ConversionRetryInterval)
		pd.FailedConversionCount++
		_, err2 := tx.Update(&pd)
		if err2 == nil {
			err2 = tx.Commit()
		}
		if err2 != nil {
			return fmt.Errorf("translation failed: %w (additional error during DB update: %s)", err, err2.Error())
		}
		return fmt.Errorf("translation failed: %w", err)
	}

	//store the translated payload
	targetPayload := string(targetPayloadBytes)
	pd.Payload = &targetPayload
	now := c.timeNow()
	pd.ConvertedAt = &now

	_, err = tx.Update(&pd)
	if err != nil {
		return err
	}
	return tx.Commit()
}
