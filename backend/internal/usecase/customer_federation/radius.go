package federation

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	logger "github.com/mmtaee/ocserv-dashboard/backend/internal/platform/logging"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var ErrRadiusRejected = errors.New("radius access rejected")

type RadiusPolicy struct {
	Group          string
	SessionTimeout uint32
}

func (m *Master) AuthorizeRadius(ctx context.Context, node, name, password, sessionID string) (RadiusPolicy, error) {
	policy := RadiusPolicy{}
	if !ValidUsername(name) || !validPassword(password) || sessionID == "" || len(sessionID) > 128 {
		return policy, ErrRadiusRejected
	}
	customer, err := m.store.ByUsername(ctx, name)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return policy, ErrRadiusRejected
	}
	if err != nil {
		return policy, err
	}
	if customer.Locked || customer.DeletedAt != nil || bcrypt.CompareHashAndPassword(customer.PasswordHash, []byte(password)) != nil {
		return policy, ErrRadiusRejected
	}
	row, policy, err := m.radiusGrant(ctx, customer, node)
	if err != nil {
		return policy, err
	}
	bound, err := m.store.BindRadiusSession(ctx, customer, row, sessionID)
	if err != nil {
		return policy, err
	}
	if !bound {
		return policy, ErrRadiusRejected
	}
	return policy, nil
}

func (m *Master) radiusGrant(ctx context.Context, customer models.PortalCustomer, node string) (models.PortalAssignment, RadiusPolicy, error) {
	var assignment models.PortalAssignment
	var policy RadiusPolicy
	if customer.Locked || customer.DeletedAt != nil {
		return assignment, policy, ErrRadiusRejected
	}
	if err := m.validateNodes(ctx, []string{node}); err != nil {
		if errors.Is(err, ErrInvalid) {
			err = ErrRadiusRejected
		}
		return assignment, policy, err
	}
	rows, err := m.store.Assignments(ctx, customer.ID)
	if err != nil {
		return assignment, policy, err
	}
	for _, row := range rows {
		if row.Node == node && row.Desired {
			assignment = row
			break
		}
	}
	if !assignment.Desired {
		return assignment, policy, ErrRadiusRejected
	}
	defaults, err := m.store.Defaults(ctx)
	if err != nil {
		return assignment, policy, err
	}
	d, err := defaultsFor(defaults, node)
	if err != nil {
		return assignment, policy, ErrRadiusRejected
	}
	now := time.Now().UTC()
	if expires := radiusExpiry(customer, d); expires != nil {
		seconds := math.Floor(expires.Sub(now).Seconds())
		if seconds <= 0 {
			return assignment, policy, ErrRadiusRejected
		}
		policy.SessionTimeout = uint32(min(seconds, math.MaxUint32))
	}
	exceeded, err := m.radiusQuotaExceeded(ctx, assignment, d, now)
	if err != nil {
		return assignment, policy, err
	}
	if exceeded {
		return assignment, policy, ErrRadiusRejected
	}
	if d.Group != "defaults" {
		policy.Group = d.Group
	}
	return assignment, policy, nil
}

func radiusExpiry(customer models.PortalCustomer, d models.PortalNodeDefault) *time.Time {
	if d.ExpiryDays <= 0 {
		return nil
	}
	expires := customer.CreatedAt.UTC().AddDate(0, 0, d.ExpiryDays)
	return &expires
}

func (m *Master) radiusQuotaExceeded(ctx context.Context, row models.PortalAssignment, d models.PortalNodeDefault, now time.Time) (bool, error) {
	if d.TrafficType == models.Free {
		return false, nil
	}
	var from *time.Time
	switch d.TrafficType {
	case models.MonthlyTransmit, models.MonthlyReceive, models.MonthlyRxTx:
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		from = &start
	}
	total, err := m.store.RadiusBandwidth(ctx, row.CustomerID, row.Node, from, nil)
	if err != nil {
		return false, err
	}
	var used float64
	switch d.TrafficType {
	case models.MonthlyTransmit, models.TotallyTransmit:
		used = total.TX
	case models.MonthlyReceive, models.TotallyReceive:
		used = total.RX
	case models.MonthlyRxTx, models.TotallyRxTx:
		used = total.RX + total.TX
	default:
		return false, ErrInvalid
	}
	return used*(1<<30) >= float64(d.TrafficSize), nil
}

type radiusTermination struct {
	customerID, node string
}

func (m *Master) AccountRadius(ctx context.Context, packet repository.RadiusAccounting) error {
	if packet.Event == models.EventHandshake {
		customer, err := m.store.ByUsername(ctx, packet.Username)
		if err != nil {
			return fmt.Errorf("find accounting customer: %w", err)
		}
		grant, _, err := m.radiusGrant(ctx, customer, packet.Node)
		if err != nil {
			return fmt.Errorf("check accounting node grant: %w", err)
		}
		bound, err := m.store.HasAuthorizedRadiusSession(ctx, grant, packet)
		if err != nil {
			return fmt.Errorf("check authorized accounting session: %w", err)
		}
		if !bound {
			return fmt.Errorf("check authorized accounting session: %w", ErrRadiusRejected)
		}
	}
	id, err := m.store.RecordRadius(ctx, packet)
	if err != nil {
		return fmt.Errorf("record accounting event: %w", err)
	}
	if id == "" || packet.Event == models.EventDisconnect {
		return nil
	}
	// Acknowledge accounting before asking Ocserv to terminate the session.
	select {
	case m.radiusTerminations <- radiusTermination{customerID: id, node: packet.Node}:
		return nil
	default:
		return fmt.Errorf("queue accounting policy check: %w", ErrUnavailable)
	}
}

func (m *Master) terminateRadiusSession(ctx context.Context, pending radiusTermination) {
	if err := m.checkRadiusSession(ctx, pending); err != nil {
		logger.Warn("unable to check RADIUS session policy for node %s: %v", pending.node, err)
	}
}

func (m *Master) checkRadiusSession(ctx context.Context, pending radiusTermination) error {
	customer, err := m.store.Customer(ctx, pending.customerID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	rows, err := m.store.Assignments(ctx, pending.customerID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.Node != pending.node {
			continue
		}
		terminate := !row.Desired || customer.Locked || customer.DeletedAt != nil
		if !terminate {
			defaults, err := m.store.Defaults(ctx)
			if err != nil {
				return err
			}
			d, err := defaultsFor(defaults, row.Node)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			if expires := radiusExpiry(customer, d); expires != nil && !now.Before(*expires) {
				terminate = true
			} else {
				terminate, err = m.radiusQuotaExceeded(ctx, row, d, now)
				if err != nil {
					return err
				}
			}
		}
		if terminate {
			_, err := m.query(ctx, row.Node, Query{Operation: "terminate", RadiusUsername: customer.Username})
			return err
		}
	}
	return nil
}
