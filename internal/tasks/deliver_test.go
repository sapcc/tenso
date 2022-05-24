/******************************************************************************
*
*  Copyright 2022 SAP SE
*
*  Licensed under the Apache License, Version 2.0 (the "License");
*  you may not use this file except in compliance with the License.
*  You may obtain a copy of the License at
*
*      http://www.apache.org/licenses/LICENSE-2.0
*
*  Unless required by applicable law or agreed to in writing, software
*  distributed under the License is distributed on an "AS IS" BASIS,
*  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
*  See the License for the specific language governing permissions and
*  limitations under the License.
*
******************************************************************************/

package tasks_test

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sapcc/go-bits/easypg"

	"github.com/sapcc/tenso/internal/tasks"
	"github.com/sapcc/tenso/internal/tenso"
	"github.com/sapcc/tenso/internal/test"
)

func TestDeliveryCommon(t *testing.T) {
	s := test.NewSetup(t,
		test.WithTaskContext,
		test.WithRoute("test-foo.v1 -> test-bar.v1"),
		test.WithRoute("test-foo.v1 -> test-baz.v1"),
	)

	//set up one event with two pending deliveries, just like `POST /v1/events/new` does it
	s.Clock.StepBy(1 * time.Hour)
	user := tenso.User{
		Name:       "testusername",
		UUID:       "testuserid",
		DomainName: "testdomainname",
	}
	test.Must(t, s.DB.Insert(&user))
	event := tenso.Event{
		CreatorID:   user.ID,
		CreatedAt:   s.Clock.Now(),
		PayloadType: "test-foo.v1",
		Payload:     `{"event":"foo","value":42}`,
		Description: "foo event with value 42",
	}
	test.Must(t, s.DB.Insert(&event))
	for _, targetPayloadType := range []string{"test-bar.v1", "test-baz.v1"} {
		test.Must(t, s.DB.Insert(&tenso.PendingDelivery{
			EventID:          event.ID,
			PayloadType:      targetPayloadType,
			Payload:          nil,
			ConvertedAt:      nil,
			NextConversionAt: s.Clock.Now(),
			NextDeliveryAt:   s.Clock.Now(),
		}))
	}

	tr, _ := easypg.NewTracker(t, s.DB.Db)

	//delivery idles until payloads are translated
	s.Clock.StepBy(5 * time.Minute)
	test.MustFail(t, tasks.ExecuteOne(s.TaskContext.PollForPendingDeliveries), sql.ErrNoRows.Error())
	tr.DBChanges().AssertEmpty()

	//GC does not touch events with pending deliveries
	test.Must(t, s.TaskContext.CollectGarbage())
	tr.DBChanges().AssertEmpty()

	//provide a translated payload, but an invalid one (we use this to simulate a delivery failure in the next step)
	_, err := s.DB.Exec(`UPDATE pending_deliveries SET payload = $1, converted_at = $2 WHERE payload_type = $3`,
		`{"event":"invalid","value":42}`, s.Clock.Now(), "test-bar.v1")
	test.Must(t, err)
	tr.DBChanges().Ignore()

	//simulate delivery failure by having provided a broken target payload
	s.Clock.StepBy(5 * time.Minute)
	test.MustFail(t,
		tasks.ExecuteOne(s.TaskContext.PollForPendingDeliveries),
		`while trying to deliver test-bar.v1 payload for event 1 ("foo event with value 42"): delivery failed: simulating failed delivery because of invalid payload`,
	)
	tr.DBChanges().AssertEqualf(`
			UPDATE pending_deliveries SET failed_deliveries = 1, next_delivery_at = %[1]d WHERE event_id = 1 AND payload_type = 'test-bar.v1';
		`,
		s.Clock.Now().Add(tasks.DeliveryRetryInterval).Unix(),
	)

	//fix target payload
	_, err = s.DB.Exec(`UPDATE pending_deliveries SET payload = $1, converted_at = $2 WHERE payload_type = $3`,
		`{"event":"bar","value":42}`, s.Clock.Now(), "test-bar.v1")
	test.Must(t, err)
	tr.DBChanges().Ignore()

	//delivery is still postponed because of previous failure, so we stall for now
	test.MustFail(t, tasks.ExecuteOne(s.TaskContext.PollForPendingDeliveries), sql.ErrNoRows.Error())

	//delivery goes through after waiting period is over
	s.Clock.StepBy(5 * time.Minute)
	test.Must(t, tasks.ExecuteOne(s.TaskContext.PollForPendingDeliveries))
	tr.DBChanges().AssertEqualf(`DELETE FROM pending_deliveries WHERE event_id = 1 AND payload_type = 'test-bar.v1';`)

	//also deliver the second payload in the same way
	_, err = s.DB.Exec(`UPDATE pending_deliveries SET payload = $1, converted_at = $2 WHERE payload_type = $3`,
		`{"event":"baz","value":42}`, s.Clock.Now(), "test-baz.v1")
	test.Must(t, err)
	test.Must(t, tasks.ExecuteOne(s.TaskContext.PollForPendingDeliveries))
	tr.DBChanges().AssertEqualf(`DELETE FROM pending_deliveries WHERE event_id = 1 AND payload_type = 'test-baz.v1';`)

	//since all payloads were delivered, GC will clean up the event
	test.Must(t, s.TaskContext.CollectGarbage())
	tr.DBChanges().AssertEqualf(`DELETE FROM events WHERE id = 1;`)
}

func TestParallelDelivery(t *testing.T) {
	//This test checks that, when there are multiple payloads to convert, each
	//conversion is executed EXACTLY ONCE.
	const eventCount = 10

	s := test.NewSetup(t,
		test.WithTaskContext,
		test.WithRoute("test-foo.v1 -> test-bar.v1"),
	)

	//set up several events with one pending delivery each
	s.Clock.StepBy(1 * time.Hour)
	user := tenso.User{
		Name:       "testusername",
		UUID:       "testuserid",
		DomainName: "testdomainname",
	}
	test.Must(t, s.DB.Insert(&user))
	for idx := 0; idx < eventCount; idx++ {
		event := tenso.Event{
			CreatorID:   user.ID,
			CreatedAt:   s.Clock.Now(),
			PayloadType: "test-foo.v1",
			Payload:     `{"event":"foo","value":42}`,
		}
		test.Must(t, s.DB.Insert(&event))
		test.Must(t, s.DB.Insert(&tenso.PendingDelivery{
			EventID:          event.ID,
			PayloadType:      "test-bar.v1",
			Payload:          p2str(`{"event":"bar","value":42}`),
			ConvertedAt:      p2time(s.Clock.Now()),
			NextConversionAt: s.Clock.Now(),
			NextDeliveryAt:   s.Clock.Now(),
		}))
	}

	tr, _ := easypg.NewTracker(t, s.DB.Db)

	//execute all deliveries in parallel
	blocker := make(chan struct{})
	s.TaskContext.Blocker = blocker
	wg := &sync.WaitGroup{}
	wg.Add(eventCount)
	for idx := 0; idx < eventCount; idx++ {
		go func() {
			defer wg.Done()
			test.Must(t, tasks.ExecuteOne(s.TaskContext.PollForPendingDeliveries))
		}()
	}

	time.Sleep(100 * time.Millisecond)
	close(blocker)
	wg.Wait()

	//check that all deliveries were completed (which implies that each goroutine
	//picked a unique delivery to work on)
	var lines []string
	for idx := 1; idx <= eventCount; idx++ {
		lines = append(lines, fmt.Sprintf(
			"DELETE FROM pending_deliveries WHERE event_id = %[1]d AND payload_type = '%[2]s';",
			idx, "test-bar.v1",
		))
	}
	sort.Strings(lines)
	tr.DBChanges().AssertEqualf(strings.Join(lines, "\n"))
}

func p2str(val string) *string {
	return &val
}

func p2time(val time.Time) *time.Time {
	return &val
}
