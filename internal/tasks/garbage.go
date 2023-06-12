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
