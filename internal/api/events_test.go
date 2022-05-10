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

package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/sapcc/go-bits/assert"
	"github.com/sapcc/go-bits/easypg"
	"github.com/sapcc/tenso/internal/test"
)

func TestPostNewEvent(t *testing.T) {
	s := test.NewSetup(t,
		test.WithAPI,
		test.WithRoute("test-foo.v1 -> test-bar.v1"),
	)

	tr, tr0 := easypg.NewTracker(t, s.DB.Db)
	tr0.AssertEmpty()

	//TODO test error cases

	//test successful event ingestion
	s.Clock.StepBy(1 * time.Minute)
	s.Validator.Allow("event:create")
	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/events/new?payload_type=test-foo.v1",
		Body:         assert.JSONObject{"foo": 42},
		ExpectStatus: http.StatusAccepted,
	}.Check(t, s.Handler)

	tr.DBChanges().AssertEqualf(`
		INSERT INTO events (id, creator_id, created_at, payload_type, payload) VALUES (1, 1, %[1]d, 'test-foo.v1', '{"foo":42}');
		INSERT INTO pending_deliveries (event_id, payload_type, payload, converted_at, failed_conversions, failed_deliveries, next_delivery_at) VALUES (1, 'test-bar.v1', NULL, NULL, 0, 0, %[1]d);
		INSERT INTO users (id, uuid, name, domain_name) VALUES (1, 'testuserid', 'testusername', 'testdomainname');
	`, s.Clock.Now().Unix())

	//test that ingestion of a second event from the same user reuses the `users` entry we just made
	s.Clock.StepBy(1 * time.Minute)
	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/events/new?payload_type=test-foo.v1",
		Body:         assert.JSONObject{"foo": 44},
		ExpectStatus: http.StatusAccepted,
	}.Check(t, s.Handler)

	tr.DBChanges().AssertEqualf(`
		INSERT INTO events (id, creator_id, created_at, payload_type, payload) VALUES (2, 1, %[1]d, 'test-foo.v1', '{"foo":44}');
		INSERT INTO pending_deliveries (event_id, payload_type, payload, converted_at, failed_conversions, failed_deliveries, next_delivery_at) VALUES (2, 'test-bar.v1', NULL, NULL, 0, 0, %[1]d);
	`, s.Clock.Now().Unix())
}
