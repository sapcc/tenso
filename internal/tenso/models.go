// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package tenso

import (
	"time"

	"github.com/go-gorp/gorp/v3"
)

// Event contains a record from the `events` table.
type Event struct {
	ID              int64     `db:"id"`
	CreatorID       int64     `db:"creator_id"` // ID into the `users` table
	CreatedAt       time.Time `db:"created_at"`
	PayloadType     string    `db:"payload_type"`
	Payload         string    `db:"payload"`
	Description     string    `db:"description"`       // a short summary that appears in log messages
	RoutingInfoJSON string    `db:"routing_info_json"` // from the X-Tenso-Routing-Info header
}

// User contains a record from the `users` table.
type User struct {
	ID         int64  `db:"id"`
	UUID       string `db:"uuid"`
	Name       string `db:"name"`
	DomainName string `db:"domain_name"`
}

// PendingDelivery contains a record from the `pending_deliveries` table.
type PendingDelivery struct {
	EventID     int64  `db:"event_id"`
	PayloadType string `db:"payload_type"`
	// Payload and ConvertedAt are nil when the payload has not been converted from event.Payload yet.
	Payload               *string    `db:"payload"`
	ConvertedAt           *time.Time `db:"converted_at"`
	FailedConversionCount int64      `db:"failed_conversions"`
	NextConversionAt      time.Time  `db:"next_conversion_at"`
	FailedDeliveryCount   int64      `db:"failed_deliveries"`
	NextDeliveryAt        time.Time  `db:"next_delivery_at"`
}

func initModels(db *gorp.DbMap) {
	db.AddTableWithName(Event{}, "events").SetKeys(true, "id")
	db.AddTableWithName(User{}, "users").SetKeys(true, "id")
	db.AddTableWithName(PendingDelivery{}, "pending_deliveries").SetKeys(false, "event_id", "payload_type")
}
