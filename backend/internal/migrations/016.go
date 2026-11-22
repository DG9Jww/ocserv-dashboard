package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var Migration016 = &gormigrate.Migration{
	ID: "016_resync_managed_node_lifecycle",
	Migrate: func(tx *gorm.DB) error {
		return tx.Exec(`
			ALTER TABLE portal_assignments ADD COLUMN IF NOT EXISTS remove boolean NOT NULL DEFAULT false;
			UPDATE portal_assignments AS a
			SET remove = (c.deleted_at IS NOT NULL),
			    desired = (a.desired AND c.deleted_at IS NULL),
			    version = a.version + 1, retry_at = NOW(), error = ''
			FROM portal_customers AS c
			WHERE a.customer_id = c.id AND (NOT a.desired OR c.deleted_at IS NOT NULL)
		`).Error
	},
	Rollback: func(tx *gorm.DB) error {
		return tx.Exec("ALTER TABLE portal_assignments DROP COLUMN remove").Error
	},
}
