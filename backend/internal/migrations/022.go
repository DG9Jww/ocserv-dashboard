package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var Migration022 = &gormigrate.Migration{
	ID: "022_radius_connection_limits",
	Migrate: func(tx *gorm.DB) error {
		return tx.Exec(`
			ALTER TABLE portal_policy_groups
				ADD COLUMN max_connections bigint NOT NULL DEFAULT 0
				CHECK (max_connections BETWEEN 0 AND 4294967295);
			ALTER TABLE portal_radius_sessions
				ADD COLUMN last_seen_at timestamptz NOT NULL DEFAULT now();
		`).Error
	},
	Rollback: func(tx *gorm.DB) error {
		return tx.Exec(`
			ALTER TABLE portal_radius_sessions DROP COLUMN last_seen_at;
			ALTER TABLE portal_policy_groups DROP COLUMN max_connections;
		`).Error
	},
}
