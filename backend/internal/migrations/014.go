package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var Migration014 = &gormigrate.Migration{
	ID: "014_portal_customer_lifecycle",
	Migrate: func(tx *gorm.DB) error {
		return tx.Exec("ALTER TABLE portal_customers ADD COLUMN locked boolean NOT NULL DEFAULT false, ADD COLUMN deleted_at timestamptz").Error
	},
	Rollback: func(tx *gorm.DB) error {
		return tx.Exec("ALTER TABLE portal_customers DROP COLUMN locked, DROP COLUMN deleted_at").Error
	},
}
