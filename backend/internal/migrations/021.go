package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var Migration021 = &gormigrate.Migration{
	ID: "021_customer_login_protection",
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`
            ALTER TABLE portal_customers
                ADD COLUMN email text NOT NULL DEFAULT '',
                ADD COLUMN login_locked_until timestamptz,
                ADD COLUMN last_login_lock_at timestamptz;
            CREATE TABLE portal_login_protections (
                id boolean PRIMARY KEY DEFAULT true CHECK (id),
                max_failures integer NOT NULL CHECK (max_failures BETWEEN 2 AND 100),
                window_minutes integer NOT NULL CHECK (window_minutes BETWEEN 1 AND 1440),
                lock_minutes integer NOT NULL CHECK (lock_minutes BETWEEN 1 AND 1440)
            );
            INSERT INTO portal_login_protections (max_failures, window_minutes, lock_minutes) VALUES (5, 15, 15);
            CREATE TABLE portal_login_failures (
                customer_id uuid NOT NULL REFERENCES portal_customers(id) ON DELETE CASCADE,
                failed_at timestamptz NOT NULL,
                request_key text,
                counted boolean NOT NULL DEFAULT true,
                UNIQUE (customer_id, request_key)
            );
            CREATE INDEX portal_login_failures_customer_time ON portal_login_failures (customer_id, failed_at);
        `).Error
	},
	Rollback: func(db *gorm.DB) error {
		return db.Exec(`
            DROP TABLE portal_login_failures, portal_login_protections;
            ALTER TABLE portal_customers DROP COLUMN email, DROP COLUMN login_locked_until, DROP COLUMN last_login_lock_at;
        `).Error
	},
}
