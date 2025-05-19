// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package tenso

import (
	"database/sql"

	"github.com/go-gorp/gorp/v3"
	"github.com/sapcc/go-bits/easypg"
)

var sqlMigrations = map[string]string{
	"001_initial.up.sql": `
		CREATE TABLE users (
			id          BIGSERIAL NOT NULL PRIMARY KEY,
			uuid        TEXT      NOT NULL UNIQUE,
			name        TEXT      NOT NULL,
			domain_name TEXT      NOT NULL
		);

		CREATE TABLE events (
			id           BIGSERIAL   NOT NULL PRIMARY KEY,
			creator_id   BIGINT      NOT NULL REFERENCES users ON DELETE RESTRICT,
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			payload_type TEXT        NOT NULL,
			payload      TEXT        NOT NULL
		);

		CREATE TABLE pending_deliveries (
			event_id           BIGINT      NOT NULL REFERENCES events ON DELETE RESTRICT,
			payload_type       TEXT        NOT NULL,
			payload            TEXT        DEFAULT NULL,
			converted_at       TIMESTAMPTZ DEFAULT NULL,
			failed_conversions INT         NOT NULL DEFAULT 0,
			next_conversion_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			failed_deliveries  INT         NOT NULL DEFAULT 0,
			next_delivery_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (event_id, payload_type)
		);
	`,
	"001_initial.down.sql": `
		DROP TABLE pending_deliveries;
		DROP TABLE events;
		DROP TABLE users;
	`,
	"002_add_events_description.up.sql": `
		ALTER TABLE events ADD COLUMN description TEXT NOT NULL DEFAULT '';
	`,
	"002_add_events_description.down.sql": `
		ALTER TABLE events DROP COLUMN description;
	`,
	"003_add_events_routing_info.up.sql": `
		ALTER TABLE events ADD COLUMN routing_info_json TEXT NOT NULL DEFAULT '';
	`,
	"003_add_events_routing_info.down.sql": `
		ALTER TABLE events DROP COLUMN routing_info_json;
	`,
}

// DBConfiguration returns the easypg.Configuration object that func main() needs to initialize the DB connection.
func DBConfiguration() easypg.Configuration {
	return easypg.Configuration{
		Migrations: sqlMigrations,
	}
}

// InitORM wraps a database connection into a gorp.DbMap instance.
func InitORM(dbConn *sql.DB) *gorp.DbMap {
	// ensure that this process does not starve other Tenso processes for DB connections
	dbConn.SetMaxOpenConns(16)

	result := &gorp.DbMap{Db: dbConn, Dialect: gorp.PostgresDialect{}}
	initModels(result)
	return result
}
