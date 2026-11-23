package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var Migration017 = &gormigrate.Migration{
	ID: "017_purge_deleted_customers",
	Migrate: func(tx *gorm.DB) error {
		return tx.Exec(`
			UPDATE portal_customers
			SET password_hash = ''::bytea, password_box = ''::bytea
			WHERE deleted_at IS NOT NULL;
			UPDATE portal_assignments AS a
			SET remove = true, desired = false, version = a.version + 1,
			    retry_at = NOW() + INTERVAL '20 seconds', error = ''
			FROM portal_customers AS c
			WHERE a.customer_id = c.id AND c.deleted_at IS NOT NULL
		`).Error
	},
	// Deleted account data and discarded credentials cannot be recreated on rollback.
	Rollback: func(tx *gorm.DB) error { return nil },
}
