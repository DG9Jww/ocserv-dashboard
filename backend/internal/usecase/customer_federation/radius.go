package federation

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	logger "github.com/mmtaee/ocserv-dashboard/backend/internal/platform/logging"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
	"gorm.io/gorm"
)

var ErrRadiusRejected = errors.New("radius access rejected")

type RadiusPolicy struct {
	SessionTimeout uint32
	Network        models.PortalPolicyGroup
}

func (m *Master) AuthorizeRadius(ctx context.Context, node, name, password, sessionID, packetVector string) (RadiusPolicy, error) {
	policy := RadiusPolicy{}
	if !ValidUsername(name) || password == "" || len(password) > 72 || sessionID == "" || len(sessionID) > 128 {
		return policy, ErrRadiusRejected
	}
	customer, err := m.store.ByUsername(ctx, name)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return policy, ErrRadiusRejected
	}
	if err != nil {
		return policy, err
	}
	if customer.Locked || customer.DeletedAt != nil {
		return policy, ErrRadiusRejected
	}
	row, policy, err := m.radiusGrant(ctx, customer, node)
	if err != nil {
		return policy, err
	}
	var requestKey *string
	vector := strings.TrimPrefix(strings.ToLower(packetVector), "0x")
	if len(vector) == 32 {
		_, err := hex.DecodeString(vector)
		if err == nil {
			key := node + ":" + vector
			requestKey = &key
		}
	}
	accepted, err := m.verifyCustomerPassword(ctx, customer, password, requestKey)
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && !accepted) {
		return policy, ErrRadiusRejected
	}
	if err != nil {
		return policy, err
	}
	if customer.PolicyGroupID != nil {
		policy.Network, err = m.store.PolicyGroup(ctx, *customer.PolicyGroupID)
		if err != nil {
			return policy, err
		}
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
	now := time.Now().UTC()
	if customer.ExpiresAt != nil {
		if !now.Before(*customer.ExpiresAt) {
			return assignment, policy, ErrRadiusRejected
		}
		seconds := math.Floor(customer.ExpiresAt.Sub(now).Seconds())
		policy.SessionTimeout = uint32(max(1, min(seconds, math.MaxUint32)))
	}
	exceeded, err := m.radiusQuotaExceeded(ctx, customer, now)
	if err != nil {
		return assignment, policy, err
	}
	if exceeded {
		return assignment, policy, ErrRadiusRejected
	}
	return assignment, policy, nil
}

func (m *Master) radiusQuotaExceeded(ctx context.Context, customer models.PortalCustomer, now time.Time) (bool, error) {
	if customer.TrafficType == models.Free {
		return false, nil
	}
	var from *time.Time
	switch customer.TrafficType {
	case models.MonthlyTransmit, models.MonthlyReceive, models.MonthlyRxTx:
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		from = &start
	}
	total, err := m.store.RadiusBandwidth(ctx, customer.ID, "", from, nil)
	if err != nil {
		return false, err
	}
	var used float64
	switch customer.TrafficType {
	case models.MonthlyTransmit, models.TotallyTransmit:
		used = total.TX
	case models.MonthlyReceive, models.TotallyReceive:
		used = total.RX
	case models.MonthlyRxTx, models.TotallyRxTx:
		used = total.RX + total.TX
	default:
		return false, ErrInvalid
	}
	return used*(1<<30) >= float64(customer.TrafficSize), nil
}

type radiusTermination struct {
	customerID, node string
}

func (m *Master) AccountRadius(ctx context.Context, packet repository.RadiusAccounting) error {
	var id string
	var err error
	if packet.Event == models.EventHandshake {
		customer, lookupErr := m.store.ByUsername(ctx, packet.Username)
		if lookupErr != nil {
			return fmt.Errorf("find accounting customer: %w", lookupErr)
		}
		grant, _, grantErr := m.radiusGrant(ctx, customer, packet.Node)
		if grantErr != nil {
			return fmt.Errorf("check accounting node grant: %w", grantErr)
		}
		id, err = m.store.StartRadiusSession(ctx, grant, packet)
		if err == nil && id == "" {
			return fmt.Errorf("check authorized accounting session: %w", ErrRadiusRejected)
		}
	} else {
		id, err = m.store.RecordRadius(ctx, packet)
	}
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
	now := time.Now().UTC()
	expired := customer.ExpiresAt != nil && !now.Before(*customer.ExpiresAt)
	blocked := customer.Locked || customer.DeletedAt != nil || expired
	if !blocked {
		blocked, err = m.radiusQuotaExceeded(ctx, customer, now)
		if err != nil {
			return err
		}
	}

	var failures error
	for _, row := range rows {
		if (blocked && row.Desired) || (row.Node == pending.node && !row.Desired) {
			if _, err := m.query(ctx, row.Node, Query{Operation: "terminate", RadiusUsername: customer.Username}); err != nil {
				failures = errors.Join(failures, fmt.Errorf("terminate sessions on node %s: %w", row.Node, err))
			}
		}
	}
	return failures
}
