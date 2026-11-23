package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var Migration018 = &gormigrate.Migration{
	ID: "018_customer_radius",
	Migrate: func(tx *gorm.DB) error {
		return tx.Exec(`
			CREATE TABLE portal_radius_sessions (
				node text NOT NULL,
				session_id text NOT NULL,
				customer_id uuid NOT NULL REFERENCES portal_customers(id) ON DELETE CASCADE,
				rx bigint NOT NULL DEFAULT 0,
				tx bigint NOT NULL DEFAULT 0,
				started boolean NOT NULL DEFAULT false,
                stopped boolean NOT NULL DEFAULT false,
				PRIMARY KEY (node, session_id)
			);
			CREATE INDEX portal_radius_sessions_customer ON portal_radius_sessions (customer_id);
			CREATE TABLE portal_radius_events (
				id uuid PRIMARY KEY DEFAULT uuidv7(),
				customer_id uuid NOT NULL REFERENCES portal_customers(id) ON DELETE CASCADE,
				node text NOT NULL,
				created_at timestamptz NOT NULL,
				event text NOT NULL,
				ip text NOT NULL,
				rx bigint NOT NULL DEFAULT 0,
				tx bigint NOT NULL DEFAULT 0
			);
			CREATE INDEX portal_radius_events_customer_time ON portal_radius_events (customer_id, created_at, id);
		`).Error
	},
	Rollback: func(tx *gorm.DB) error {
		return tx.Exec("DROP TABLE portal_radius_events, portal_radius_sessions").Error
	},
}
