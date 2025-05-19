// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package tasks

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sapcc/go-bits/jobloop"
	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/go-bits/sqlext"
)

var gcDeliveredEventsQuery = sqlext.SimplifyWhitespace(`
	DELETE FROM events WHERE id NOT IN (SELECT event_id FROM pending_deliveries)
`)

func (c *Context) GarbageCollectionJob(registerer prometheus.Registerer) jobloop.Job {
	return (&jobloop.CronJob{
		Metadata: jobloop.JobMetadata{
			ReadableName: "garbage collection",
			CounterOpts: prometheus.CounterOpts{
				Name: "tenso_garbage_collections",
				Help: "Counter for database GC runs.",
			},
		},
		Interval: 5 * time.Minute,
		Task:     c.collectGarbage,
	}).Setup(registerer)
}

func (c *Context) collectGarbage(_ context.Context, _ prometheus.Labels) error {
	result, err := c.DB.Exec(gcDeliveredEventsQuery)
	if err != nil {
		return err
	}
	numDeleted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if numDeleted > 0 {
		logg.Info("cleaned up %d fully-delivered events", numDeleted)
	}

	return nil
}
