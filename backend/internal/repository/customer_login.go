package repository

import (
	"bytes"
	"context"
	"time"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *PortalRepository) LoginProtection(ctx context.Context) (models.PortalLoginProtection, error) {
	var policy models.PortalLoginProtection
	err := r.db.WithContext(ctx).First(&policy, "id = true").Error
	return policy, err
}

func (r *PortalRepository) UpdateLoginProtection(ctx context.Context, policy models.PortalLoginProtection) error {
	return r.db.WithContext(ctx).Model(&models.PortalLoginProtection{}).Where("id = true").Updates(policy).Error
}

// RecordCustomerLogin serializes password results across all login entry points for one account.
func (r *PortalRepository) RecordCustomerLogin(ctx context.Context, customer models.PortalCustomer, passwordOK bool, requestKey *string) (accepted, locked bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.PortalCustomer
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "password_hash", "locked", "deleted_at", "login_locked_until").
			First(&current, "id = ?", customer.ID).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		if current.Locked || current.DeletedAt != nil || !bytes.Equal(current.PasswordHash, customer.PasswordHash) ||
			(current.LoginLockedUntil != nil && now.Before(*current.LoginLockedUntil)) {
			return nil
		}
		if passwordOK || current.LoginLockedUntil != nil {
			if err := resetCustomerLogin(tx, current.ID); err != nil {
				return err
			}
		}
		if passwordOK {
			accepted = true
			return nil
		}

		var policy models.PortalLoginProtection
		if err := tx.First(&policy, "id = true").Error; err != nil {
			return err
		}
		// Keep request keys long enough for retransmissions even after resetting the count.
		retention := time.Duration(max(policy.WindowMinutes, 15)) * time.Minute
		if err := tx.Exec("DELETE FROM portal_login_failures WHERE customer_id = ? AND failed_at <= ?", current.ID, now.Add(-retention)).Error; err != nil {
			return err
		}
		result := tx.Exec(`
            INSERT INTO portal_login_failures (customer_id, failed_at, request_key)
            VALUES (?, ?, ?) ON CONFLICT (customer_id, request_key) DO NOTHING
        `, current.ID, now, requestKey)
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		var count int64
		windowStart := now.Add(-time.Duration(policy.WindowMinutes) * time.Minute)
		if err := tx.Table("portal_login_failures").Where("customer_id = ? AND counted AND failed_at > ?", current.ID, windowStart).Count(&count).Error; err != nil {
			return err
		}
		if count < int64(policy.MaxFailures) {
			return nil
		}
		until := now.Add(time.Duration(policy.LockMinutes) * time.Minute)
		locked = true
		return tx.Model(&current).Updates(map[string]any{"login_locked_until": until, "last_login_lock_at": now}).Error
	})
	return
}

func (r *PortalRepository) ResetCustomerLogin(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var customer models.PortalCustomer
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").First(&customer, "id = ? AND deleted_at IS NULL", id).Error; err != nil {
			return err
		}
		return resetCustomerLogin(tx, id)
	})
}

func resetCustomerLogin(tx *gorm.DB, id string) error {
	if err := tx.Model(&models.PortalCustomer{}).Where("id = ? AND login_locked_until IS NOT NULL", id).
		Update("login_locked_until", nil).Error; err != nil {
		return err
	}
	return tx.Exec("UPDATE portal_login_failures SET counted = false WHERE customer_id = ? AND counted", id).Error
}
