package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"gorm.io/gorm"
	"time"
)

// Migration 020 removes the original node-grant schema after checking that it is empty.
type portalCustomer013 struct {
	ID           string   `gorm:"type:uuid;primaryKey;default:uuidv7()"`
	Username     string   `gorm:"not null;uniqueIndex"`
	PasswordHash []byte   `gorm:"not null"`
	Nodes        []string `gorm:"serializer:json;type:jsonb"`
	Groups       []string `gorm:"serializer:json;type:jsonb"`
	DefaultNode  string
	Locked       bool
	DeletedAt    *time.Time
	CreatedAt    time.Time
}

func (portalCustomer013) TableName() string { return "portal_customers" }

type portalNodeGroup013 struct {
	ID    string   `gorm:"type:uuid;primaryKey;default:uuidv7()"`
	Name  string   `gorm:"not null;uniqueIndex"`
	Nodes []string `gorm:"serializer:json;type:jsonb"`
}

func (portalNodeGroup013) TableName() string { return "portal_node_groups" }

type portalNodeDefault013 struct {
	Node        string `gorm:"primaryKey"`
	Group       string
	TrafficType models.TrafficType
	TrafficSize int64
	ExpiryDays  int
}

func (portalNodeDefault013) TableName() string { return "portal_node_defaults" }

var portalTables = []any{&portalCustomer013{}, &portalNodeGroup013{}, &portalNodeDefault013{}, &models.PortalAssignment{}}

var Migration013 = &gormigrate.Migration{
	ID:       "013_unified_customer_portal",
	Migrate:  func(tx *gorm.DB) error { return tx.AutoMigrate(portalTables...) },
	Rollback: func(tx *gorm.DB) error { return tx.Migrator().DropTable(portalTables...) },
}
