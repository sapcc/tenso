// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package tenso

import (
	"context"
	"os"

	"github.com/dlmiddlecote/sqlstats"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sapcc/go-api-declarations/bininfo"
	"github.com/sapcc/go-bits/must"
	"github.com/sapcc/go-bits/osext"
	"go.xyrillian.de/gg/gsql"
	"go.xyrillian.de/gg/pgruntime"

	// include DB driver
	_ "github.com/lib/pq"
)

var sqlMigrations = map[int64]string{
	1: `
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
	2: `
		ALTER TABLE events ADD COLUMN description TEXT NOT NULL DEFAULT '';
	`,
	3: `
		ALTER TABLE events ADD COLUMN routing_info_json TEXT NOT NULL DEFAULT '';
	`,
}

// DBConfiguration returns the [pgruntime.ConnectionBehavior] object that func main() needs to initialize the DB connection.
func DBConfiguration() pgruntime.ConnectionBehavior {
	return pgruntime.ConnectionBehavior{
		Migrations: sqlMigrations,
	}
}

// InitDB initializes a DB connection for productive use.
// (Tests use the DB connection logic in test.NewSetup() instead.)
func InitDB(ctx context.Context) *gsql.DB {
	target := pgruntime.ConnectionTarget{
		HostName:          osext.GetenvOrDefault("TENSO_DB_HOSTNAME", "localhost"),
		Port:              osext.GetenvOrDefault("TENSO_DB_PORT", "5432"),
		UserName:          osext.GetenvOrDefault("TENSO_DB_USERNAME", "postgres"),
		Password:          os.Getenv("TENSO_DB_PASSWORD"),
		ConnectionOptions: os.Getenv("TENSO_DB_CONNECTION_OPTIONS"),
		DatabaseName:      osext.GetenvOrDefault("TENSO_DB_NAME", "tenso"),
		ApplicationName:   bininfo.Component(),
	}
	dbConn := must.Return(pgruntime.StdConnector("postgres").Connect(ctx, target, DBConfiguration()))

	// ensure that this process does not starve other Tenso processes for DB connections
	dbConn.SetMaxOpenConns(16)

	prometheus.MustRegister(sqlstats.NewStatsCollector(target.DatabaseName, dbConn))
	return dbConn
}
