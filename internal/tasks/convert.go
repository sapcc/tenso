// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package tasks

import (
	"context"
	"encoding/json"
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
var selectNextConversionQuery = sqlext.SimplifyWhitespace(`
	SELECT * FROM pending_deliveries
	 WHERE converted_at IS NULL AND next_conversion_at <= $1
	 ORDER BY next_conversion_at ASC, payload_type ASC   -- secondary order ensures deterministic behavior during test
	 LIMIT 1 FOR UPDATE SKIP LOCKED
`)

const (
	ConversionRetryInterval = 2 * time.Minute
)

func (c *Context) ConversionJob(registerer prometheus.Registerer) jobloop.Job {
	return (&jobloop.TxGuardedJob[*gorp.Transaction, tenso.PendingDelivery]{
		Metadata: jobloop.JobMetadata{
			ReadableName:    "Event conversion",
			ConcurrencySafe: true, // because "FOR UPDATE SKIP LOCKED" is used
			CounterOpts: prometheus.CounterOpts{
				Name: "tenso_event_conversions",
				Help: "Counter for conversions of event payloads.",
			},
			CounterLabels: []string{"source_payload_type", "target_payload_type"},
		},
		BeginTx: c.DB.Begin,
		DiscoverRow: func(_ context.Context, tx *gorp.Transaction, _ prometheus.Labels) (pd tenso.PendingDelivery, err error) {
			err = tx.SelectOne(&pd, selectNextConversionQuery, c.timeNow())
			return pd, err
		},
		ProcessRow: c.processConversion,
	}).Setup(registerer)
}

func (c *Context) processConversion(_ context.Context, tx *gorp.Transaction, pd tenso.PendingDelivery, labels prometheus.Labels) (returnedError error) {
	var event tenso.Event

	labels["target_payload_type"] = pd.PayloadType

	defer func() {
		if returnedError == nil {
			logg.Info("converted payload for event %d (%q) into %s", pd.EventID, event.Description, pd.PayloadType)
		} else {
			returnedError = fmt.Errorf("while trying to convert payload for event %d (%q) into %s: %w", pd.EventID, event.Description, pd.PayloadType, returnedError)
		}
	}()

	// find the corresponding event
	err := tx.SelectOne(&event, `SELECT * FROM events WHERE id = $1`, pd.EventID)
	if err != nil {
		return err
	}

	var routingInfo map[string]string
	if event.RoutingInfoJSON != "" {
		err := json.Unmarshal([]byte(event.RoutingInfoJSON), &routingInfo)
		if err != nil {
			return fmt.Errorf("while parsing event routing info: %w", err)
		}
	}

	labels["source_payload_type"] = event.PayloadType

	// find the translation handler
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

	// try to translate the payload, or set up a delayed retry on failure
	targetPayloadBytes, err := th.TranslatePayload([]byte(event.Payload), routingInfo)
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

	// store the translated payload
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
