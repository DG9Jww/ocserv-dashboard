package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var Migration014 = &gormigrate.Migration{
	ID: "014_portal_customer_lifecycle",
	Migrate: func(tx *gorm.DB) error {
		// A fresh installation may already have these columns through AutoMigrate in 013.
		return tx.Exec("ALTER TABLE portal_customers ADD COLUMN IF NOT EXISTS locked boolean NOT NULL DEFAULT false, ADD COLUMN IF NOT EXISTS deleted_at timestamptz").Error
	},
	Rollback: func(tx *gorm.DB) error {
		return tx.Exec("ALTER TABLE portal_customers DROP COLUMN locked, DROP COLUMN deleted_at").Error
	},
}
