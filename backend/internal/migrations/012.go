package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"gorm.io/gorm"
)

var Migration012 = &gormigrate.Migration{
	ID: "012_managed_customer_accounts",
	Migrate: func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&models.PortalLocalAccount{}); err != nil {
			return err
		}
		return tx.Exec("ALTER TABLE ocserv_user_session_logs ADD COLUMN portal_account_id bigint; CREATE INDEX idx_ocserv_user_session_logs_portal_account_id ON ocserv_user_session_logs(portal_account_id)").Error
	},
	Rollback: func(tx *gorm.DB) error {
		if err := tx.Exec("ALTER TABLE ocserv_user_session_logs DROP COLUMN portal_account_id").Error; err != nil {
			return err
		}
		return tx.Migrator().DropTable(&models.PortalLocalAccount{})
	},
}
