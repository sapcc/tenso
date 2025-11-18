// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/sapcc/go-bits/assert"
	"github.com/sapcc/go-bits/easypg"
	"github.com/sapcc/go-bits/httptest"

	"github.com/sapcc/tenso/internal/test"
)

func TestMain(m *testing.M) {
	easypg.WithTestDB(m, func() int { return m.Run() })
}

func TestPostNewEvent(t *testing.T) {
	s := test.NewSetup(t,
		test.WithAPI,
		test.WithRoute("test-foo.v1 -> test-bar.v1"),
		test.WithRoute("test-foo.v1 -> test-baz.v1"),
	)
	h := s.Handler
	ctx := t.Context()

	tr, tr0 := easypg.NewTracker(t, s.DB.Db)
	tr0.AssertEmpty()

	// test error cases: invalid payload type
	body := map[string]any{"event": "foo", "value": 42}
	invalidPayloadTypes := map[string]string{
		"": `need exactly one value for query parameter "payload_type"`,
		"?payload_type=test-foo.v1&payload_type=test-bar.v1": `need exactly one value for query parameter "payload_type"`,
		"?payload_type=what!?":                               `invalid value provided for query parameter "payload_type"`,
		"?payload_type=test-bar.v1":                          `cannot accept events with payload_type "test-bar.v1"`,
	}
	for queryString, expectedError := range invalidPayloadTypes {
		t.Run(queryString, func(t *testing.T) {
			h.RespondTo(ctx, "POST /v1/events/new"+queryString,
				httptest.WithJSONBody(body),
			).ExpectText(t, http.StatusBadRequest, expectedError+"\n")
		})
	}

	// test error cases: invalid payload
	h.RespondTo(ctx, "POST /v1/events/new?payload_type=test-foo.v1",
		httptest.WithJSONBody(map[string]any{"event": "bar", "value": 42}),
	).ExpectText(t, http.StatusUnprocessableEntity,
		"invalid event payload: expected event = \"foo\", but got \"bar\"\n",
	)

	// test error cases: no permission
	s.Validator.Enforcer.Forbid("event:create")
	resp := h.RespondTo(ctx, "POST /v1/events/new?payload_type=test-foo.v1",
		httptest.WithJSONBody(body),
	)
	assert.Equal(t, resp.StatusCode(), http.StatusForbidden)
	s.Validator.Enforcer.Allow("event:create")

	// test error cases: malformed X-Tenso-Routing-Info header
	for _, invalidKeyValuePair := range []string{"target-foobar", "target=", "=foobar"} {
		t.Run(invalidKeyValuePair, func(t *testing.T) {
			h.RespondTo(ctx, "POST /v1/events/new?payload_type=test-foo.v1",
				httptest.WithJSONBody(body),
				httptest.WithHeader("X-Tenso-Routing-Info", invalidKeyValuePair+", priority=42"),
			).ExpectText(t, http.StatusBadRequest,
				fmt.Sprintf("invalid routing info: expected a \"key=value\" pair, but found %q\n", invalidKeyValuePair),
			)
		})
	}

	h.RespondTo(ctx, "POST /v1/events/new?payload_type=test-foo.v1",
		httptest.WithJSONBody(body),
		httptest.WithHeader("X-Tenso-Routing-Info", "target=foo, target=bar, priority=42"),
	).ExpectText(t, http.StatusBadRequest,
		"invalid routing info: multiple values for key \"target\"\n",
	)

	// since we only tested error cases so far, nothing should have been written into the DB
	tr.DBChanges().AssertEmpty()

	// test successful event ingestion
	s.Clock.StepBy(1 * time.Minute)
	resp = h.RespondTo(ctx, "POST /v1/events/new?payload_type=test-foo.v1",
		httptest.WithJSONBody(body),
	)
	assert.Equal(t, resp.StatusCode(), http.StatusAccepted)

	tr.DBChanges().AssertEqualf(`
		INSERT INTO events (id, creator_id, created_at, payload_type, payload, description, routing_info_json) VALUES (1, 1, %[1]d, 'test-foo.v1', '{"event":"foo","value":42}', 'foo event with value 42', '{}');
		INSERT INTO pending_deliveries (event_id, payload_type, next_conversion_at, next_delivery_at) VALUES (1, 'test-bar.v1', %[1]d, %[1]d);
		INSERT INTO pending_deliveries (event_id, payload_type, next_conversion_at, next_delivery_at) VALUES (1, 'test-baz.v1', %[1]d, %[1]d);
		INSERT INTO users (id, uuid, name, domain_name) VALUES (1, 'testuserid', 'testusername', 'testdomainname');
	`, s.Clock.Now().Unix())

	// test that ingestion of a second event from the same user reuses the `users` entry we just made;
	// also this event includes routing info
	s.Clock.StepBy(1 * time.Minute)
	resp = h.RespondTo(ctx, "POST /v1/events/new?payload_type=test-foo.v1",
		httptest.WithJSONBody(map[string]any{"event": "foo", "value": 44}),
		httptest.WithHeader("X-Tenso-Routing-Info", ",,, target=foobar, priority  = 42  "),
	)
	assert.Equal(t, resp.StatusCode(), http.StatusAccepted)

	tr.DBChanges().AssertEqualf(`
		INSERT INTO events (id, creator_id, created_at, payload_type, payload, description, routing_info_json) VALUES (2, 1, %[1]d, 'test-foo.v1', '{"event":"foo","value":44}', 'foo event with value 44', '{"priority":"42","target":"foobar"}');
		INSERT INTO pending_deliveries (event_id, payload_type, next_conversion_at, next_delivery_at) VALUES (2, 'test-bar.v1', %[1]d, %[1]d);
		INSERT INTO pending_deliveries (event_id, payload_type, next_conversion_at, next_delivery_at) VALUES (2, 'test-baz.v1', %[1]d, %[1]d);
	`, s.Clock.Now().Unix())
}
