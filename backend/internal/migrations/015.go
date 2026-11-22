package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var Migration015 = &gormigrate.Migration{
	ID: "015_managed_node_lifecycle",
	Migrate: func(tx *gorm.DB) error {
		return tx.Exec(`
			ALTER TABLE ocserv_users ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
			ALTER TABLE portal_local_accounts ADD COLUMN IF NOT EXISTS lock_applied boolean NOT NULL DEFAULT false
		`).Error
	},
	Rollback: func(tx *gorm.DB) error {
		return tx.Exec(`
			ALTER TABLE ocserv_users DROP COLUMN deleted_at;
			ALTER TABLE portal_local_accounts DROP COLUMN lock_applied
		`).Error
	},
}
