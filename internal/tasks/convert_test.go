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
	"testing"
	"time"

	"github.com/sapcc/go-bits/easypg"
	"github.com/sapcc/go-bits/jobloop"

	"github.com/sapcc/tenso/internal/tasks"
	"github.com/sapcc/tenso/internal/tenso"
	"github.com/sapcc/tenso/internal/test"
)

func TestConversionCommon(t *testing.T) {
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
		Payload:     `{"event":"invalid","value":42}`, //see below for why "invalid"
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
	conversionJob := s.TaskContext.ConversionJob(s.Registry)

	//simulate conversion failure by having provided a broken source payload
	s.Clock.StepBy(5 * time.Minute)
	test.MustFail(t,
		conversionJob.ProcessOne(s.Ctx),
		`could not process task for job "Event conversion": while trying to convert payload for event 1 ("foo event with value 42") into test-bar.v1: translation failed: expected event = "foo", but got "invalid"`,
	)
	tr.DBChanges().AssertEqualf(`
			UPDATE pending_deliveries SET failed_conversions = 1, next_conversion_at = %[1]d WHERE event_id = 1 AND payload_type = 'test-bar.v1';
		`,
		s.Clock.Now().Add(tasks.ConversionRetryInterval).Unix(),
	)

	//fix source payload to enable a successful conversion
	_, err := s.DB.Exec(`UPDATE events SET payload = $1`, `{"event":"foo","value":42}`)
	test.Must(t, err)
	tr.DBChanges().Ignore()

	//check successful conversion (this touches the second PendingDelivery since it's NextConversionAt is lower)
	test.Must(t, conversionJob.ProcessOne(s.Ctx))
	tr.DBChanges().AssertEqualf(`
			UPDATE pending_deliveries SET payload = '%[1]s', converted_at = %[2]d WHERE event_id = 1 AND payload_type = 'test-baz.v1';
		`,
		`{"event":"baz","value":42}`,
		s.Clock.Now().Unix(),
	)

	//second conversion is still postponed, so we stall for now
	test.MustFail(t, conversionJob.ProcessOne(s.Ctx), sql.ErrNoRows.Error())

	//second conversion goes through after waiting period is over
	s.Clock.StepBy(5 * time.Minute)
	test.Must(t, conversionJob.ProcessOne(s.Ctx))
	tr.DBChanges().AssertEqualf(`
			UPDATE pending_deliveries SET payload = '%[1]s', converted_at = %[2]d WHERE event_id = 1 AND payload_type = 'test-bar.v1';
		`,
		`{"event":"bar","value":42}`,
		s.Clock.Now().Unix(),
	)

	//nothing left to convert now
	test.MustFail(t, conversionJob.ProcessOne(s.Ctx), sql.ErrNoRows.Error())
	tr.DBChanges().AssertEmpty()
}

func TestParallelConversion(t *testing.T) {
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
			Payload:          nil,
			ConvertedAt:      nil,
			NextConversionAt: s.Clock.Now(),
			NextDeliveryAt:   s.Clock.Now(),
		}))
	}

	tr, _ := easypg.NewTracker(t, s.DB.Db)
	conversionJob := s.TaskContext.ConversionJob(s.Registry)

	test.Must(t, jobloop.ProcessMany(conversionJob, s.Ctx, eventCount))

	//check that all deliveries got their payloads converted (which implies that
	//each goroutine picked a unique delivery to work on)
	var lines []string
	for idx := 1; idx <= eventCount; idx++ {
		lines = append(lines, fmt.Sprintf(
			"UPDATE pending_deliveries SET payload = '%[1]s', converted_at = %[2]d WHERE event_id = %[3]d AND payload_type = '%[4]s';",
			`{"event":"bar","value":42}`,
			s.Clock.Now().Unix(),
			idx, "test-bar.v1",
		))
	}
	sort.Strings(lines)
	tr.DBChanges().AssertEqualf(strings.Join(lines, "\n"))
}
