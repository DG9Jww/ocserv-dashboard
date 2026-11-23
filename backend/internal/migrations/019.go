package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var Migration019 = &gormigrate.Migration{
	ID: "019_customer_radius_policy_groups",
	Migrate: func(tx *gorm.DB) error {
		return tx.Exec(`
			CREATE TABLE portal_policy_groups (
				id uuid PRIMARY KEY DEFAULT uuidv7(),
				name text NOT NULL UNIQUE,
				primary_dns text NOT NULL DEFAULT '',
				secondary_dns text NOT NULL DEFAULT '',
				routes jsonb NOT NULL DEFAULT '[]',
				upload_kb_per_second bigint NOT NULL DEFAULT 0 CHECK (upload_kb_per_second BETWEEN 0 AND 4294967295),
				download_kb_per_second bigint NOT NULL DEFAULT 0 CHECK (download_kb_per_second BETWEEN 0 AND 4294967295)
			);
			ALTER TABLE portal_customers ADD COLUMN IF NOT EXISTS policy_group_id uuid;
			ALTER TABLE portal_customers ADD CONSTRAINT fk_portal_customer_policy_group
				FOREIGN KEY (policy_group_id) REFERENCES portal_policy_groups(id) ON DELETE RESTRICT;
			CREATE INDEX idx_portal_customers_policy_group ON portal_customers(policy_group_id);
		`).Error
	},
	Rollback: func(tx *gorm.DB) error {
		return tx.Exec(`
			ALTER TABLE portal_customers DROP COLUMN policy_group_id;
			DROP TABLE portal_policy_groups;
		`).Error
	},
}
