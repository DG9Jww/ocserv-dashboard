package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var Migration020 = &gormigrate.Migration{
	ID: "020_unified_radius_authorization",
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return tx.Exec(`
                LOCK TABLE portal_customers, portal_policy_groups IN ACCESS EXCLUSIVE MODE;
                DO $$
                BEGIN
                    IF EXISTS (SELECT 1 FROM portal_customers) OR EXISTS (SELECT 1 FROM portal_policy_groups) THEN
                        RAISE EXCEPTION 'Clear existing customers and Radius Policies before upgrading to unified RADIUS authorization';
                    END IF;
                END $$;
                DELETE FROM portal_assignments;
                ALTER TABLE portal_customers
                    ADD COLUMN node text NOT NULL DEFAULT '',
                    ADD COLUMN traffic_type text NOT NULL DEFAULT 'Free',
                    ADD COLUMN traffic_size bigint NOT NULL DEFAULT 0,
                    ADD COLUMN expires_at timestamptz,
                    ADD CONSTRAINT portal_customer_one_grant CHECK (deleted_at IS NOT NULL OR ((node <> '') <> (policy_group_id IS NOT NULL))),
                    DROP COLUMN nodes,
                    DROP COLUMN groups,
                    DROP COLUMN IF EXISTS password_box;
                ALTER TABLE portal_policy_groups ADD COLUMN nodes jsonb NOT NULL DEFAULT '[]';
                DROP TABLE portal_node_groups, portal_node_defaults;
            `).Error
		})
	},
	Rollback: func(tx *gorm.DB) error {
		return tx.Transaction(func(db *gorm.DB) error {
			if err := db.Exec(`
                LOCK TABLE portal_customers, portal_policy_groups IN ACCESS EXCLUSIVE MODE;
                DO $$
                BEGIN
                    IF EXISTS (SELECT 1 FROM portal_customers) OR EXISTS (SELECT 1 FROM portal_policy_groups) THEN
                        RAISE EXCEPTION 'Clear existing customers and Radius Policies before rolling back unified RADIUS authorization';
                    END IF;
                END $$;
                ALTER TABLE portal_customers
                    DROP CONSTRAINT portal_customer_one_grant,
                    DROP COLUMN node,
                    DROP COLUMN traffic_type,
                    DROP COLUMN traffic_size,
                    DROP COLUMN expires_at,
                    ADD COLUMN nodes jsonb,
                    ADD COLUMN groups jsonb;
                ALTER TABLE portal_policy_groups DROP COLUMN nodes;
            `).Error; err != nil {
				return err
			}
			return db.AutoMigrate(&portalNodeGroup013{}, &portalNodeDefault013{})
		})
	},
}
