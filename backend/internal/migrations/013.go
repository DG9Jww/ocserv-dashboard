package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"gorm.io/gorm"
)

var portalTables = []any{&models.PortalCustomer{}, &models.PortalNodeGroup{}, &models.PortalNodeDefault{}, &models.PortalAssignment{}, &models.PortalSigningCA{}, &models.PortalClientCertificate{}}

var Migration013 = &gormigrate.Migration{
	ID:       "013_unified_customer_portal",
	Migrate:  func(tx *gorm.DB) error { return tx.AutoMigrate(portalTables...) },
	Rollback: func(tx *gorm.DB) error { return tx.Migrator().DropTable(portalTables...) },
}
