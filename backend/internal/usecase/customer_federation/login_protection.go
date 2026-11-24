package federation

import (
	"context"
	"errors"
	"time"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	logger "github.com/mmtaee/ocserv-dashboard/backend/internal/platform/logging"
	"golang.org/x/crypto/bcrypt"
)

func (m *Master) LoginProtection(ctx context.Context) (models.PortalLoginProtection, error) {
	return m.store.LoginProtection(ctx)
}

func (m *Master) UpdateLoginProtection(ctx context.Context, policy models.PortalLoginProtection) error {
	if policy.MaxFailures < 2 || policy.MaxFailures > 100 || policy.WindowMinutes < 1 ||
		policy.WindowMinutes > 1440 || policy.LockMinutes < 1 || policy.LockMinutes > 1440 {
		return ErrInvalid
	}
	return m.store.UpdateLoginProtection(ctx, policy)
}

func (m *Master) UnlockCustomerLogin(ctx context.Context, id string) error {
	if !ValidID(id) {
		return ErrInvalid
	}
	if err := m.store.ResetCustomerLogin(ctx, id); err != nil {
		return err
	}
	logger.Info("customer login protection reset for account %s", id)
	return nil
}

func (m *Master) verifyCustomerPassword(ctx context.Context, customer models.PortalCustomer, password string, requestKey *string) (bool, error) {
	if len(password) > 72 || (customer.LoginLockedUntil != nil && time.Now().Before(*customer.LoginLockedUntil)) {
		return false, nil
	}
	passwordErr := bcrypt.CompareHashAndPassword(customer.PasswordHash, []byte(password))
	if passwordErr != nil && !errors.Is(passwordErr, bcrypt.ErrMismatchedHashAndPassword) {
		return false, passwordErr
	}
	accepted, locked, err := m.store.RecordCustomerLogin(ctx, customer, passwordErr == nil, requestKey)
	if err == nil && locked {
		logger.Warn("customer temporarily locked after failed password attempts: account %s", customer.ID)
	}
	return accepted, err
}
