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
		test.WithRoute("test-foo.v1 -> test-baz.v1"),
	)

	tr, tr0 := easypg.NewTracker(t, s.DB.Db)
	tr0.AssertEmpty()

	// test error cases: invalid payload type
	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/events/new",
		Body:         assert.JSONObject{"event": "foo", "value": 42},
		ExpectStatus: http.StatusBadRequest,
		ExpectBody:   assert.StringData("need exactly one value for query parameter \"payload_type\"\n"),
	}.Check(t, s.Handler)
	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/events/new?payload_type=test-foo.v1&payload_type=test-bar.v1",
		Body:         assert.JSONObject{"event": "foo", "value": 42},
		ExpectStatus: http.StatusBadRequest,
		ExpectBody:   assert.StringData("need exactly one value for query parameter \"payload_type\"\n"),
	}.Check(t, s.Handler)
	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/events/new?payload_type=what!?",
		Body:         assert.JSONObject{"event": "foo", "value": 42},
		ExpectStatus: http.StatusBadRequest,
		ExpectBody:   assert.StringData("invalid value provided for query parameter \"payload_type\"\n"),
	}.Check(t, s.Handler)
	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/events/new?payload_type=test-bar.v1",
		Body:         assert.JSONObject{"event": "foo", "value": 42},
		ExpectStatus: http.StatusBadRequest,
		ExpectBody:   assert.StringData("cannot accept events with payload_type \"test-bar.v1\"\n"),
	}.Check(t, s.Handler)

	// test error cases: invalid payload
	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/events/new?payload_type=test-foo.v1",
		Body:         assert.JSONObject{"event": "bar", "value": 42},
		ExpectStatus: http.StatusUnprocessableEntity,
		ExpectBody:   assert.StringData("invalid event payload: expected event = \"foo\", but got \"bar\"\n"),
	}.Check(t, s.Handler)

	// test error cases: no permission
	s.Validator.Enforcer.Forbid("event:create")
	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/events/new?payload_type=test-foo.v1",
		Body:         assert.JSONObject{"event": "foo", "value": 42},
		ExpectStatus: http.StatusForbidden,
	}.Check(t, s.Handler)
	s.Validator.Enforcer.Allow("event:create")

	// test error cases: malformed X-Tenso-Routing-Info header
	for _, invalidKeyValuePair := range []string{"target-foobar", "target=", "=foobar"} {
		assert.HTTPRequest{
			Method:       "POST",
			Path:         "/v1/events/new?payload_type=test-foo.v1",
			Body:         assert.JSONObject{"event": "foo", "value": 42},
			Header:       map[string]string{"X-Tenso-Routing-Info": invalidKeyValuePair + ", priority=42"},
			ExpectStatus: http.StatusBadRequest,
			ExpectBody:   assert.StringData("invalid routing info: expected a \"key=value\" pair, but found \"" + invalidKeyValuePair + "\"\n"),
		}.Check(t, s.Handler)
	}
	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/events/new?payload_type=test-foo.v1",
		Body:         assert.JSONObject{"event": "foo", "value": 42},
		Header:       map[string]string{"X-Tenso-Routing-Info": "target=foo, target=bar, priority=42"},
		ExpectStatus: http.StatusBadRequest,
		ExpectBody:   assert.StringData("invalid routing info: multiple values for key \"target\"\n"),
	}.Check(t, s.Handler)

	// test successful event ingestion
	s.Clock.StepBy(1 * time.Minute)
	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/events/new?payload_type=test-foo.v1",
		Body:         assert.JSONObject{"event": "foo", "value": 42},
		ExpectStatus: http.StatusAccepted,
	}.Check(t, s.Handler)

	tr.DBChanges().AssertEqualf(`
		INSERT INTO events (id, creator_id, created_at, payload_type, payload, description, routing_info_json) VALUES (1, 1, %[1]d, 'test-foo.v1', '{"event":"foo","value":42}', 'foo event with value 42', '{}');
		INSERT INTO pending_deliveries (event_id, payload_type, next_conversion_at, next_delivery_at) VALUES (1, 'test-bar.v1', %[1]d, %[1]d);
		INSERT INTO pending_deliveries (event_id, payload_type, next_conversion_at, next_delivery_at) VALUES (1, 'test-baz.v1', %[1]d, %[1]d);
		INSERT INTO users (id, uuid, name, domain_name) VALUES (1, 'testuserid', 'testusername', 'testdomainname');
	`, s.Clock.Now().Unix())

	// test that ingestion of a second event from the same user reuses the `users` entry we just made;
	// also this event includes routing info
	s.Clock.StepBy(1 * time.Minute)
	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/events/new?payload_type=test-foo.v1",
		Body:         assert.JSONObject{"event": "foo", "value": 44},
		Header:       map[string]string{"X-Tenso-Routing-Info": ",,, target=foobar, priority  = 42  "},
		ExpectStatus: http.StatusAccepted,
	}.Check(t, s.Handler)

	tr.DBChanges().AssertEqualf(`
		INSERT INTO events (id, creator_id, created_at, payload_type, payload, description, routing_info_json) VALUES (2, 1, %[1]d, 'test-foo.v1', '{"event":"foo","value":44}', 'foo event with value 44', '{"priority":"42","target":"foobar"}');
		INSERT INTO pending_deliveries (event_id, payload_type, next_conversion_at, next_delivery_at) VALUES (2, 'test-bar.v1', %[1]d, %[1]d);
		INSERT INTO pending_deliveries (event_id, payload_type, next_conversion_at, next_delivery_at) VALUES (2, 'test-baz.v1', %[1]d, %[1]d);
	`, s.Clock.Now().Unix())
}
