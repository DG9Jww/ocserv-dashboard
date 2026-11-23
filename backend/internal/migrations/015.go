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
		`).Error
	},
	Rollback: func(tx *gorm.DB) error {
		return tx.Exec(`
			ALTER TABLE ocserv_users DROP COLUMN deleted_at;
		`).Error
	},
}
