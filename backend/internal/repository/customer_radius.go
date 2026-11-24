package repository

import (
	"context"
	"errors"
	"time"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type RadiusAccounting struct {
	Node, SessionID, Username, IP string
	Event                         models.OcservUserSessionEvent
	At                            time.Time
	RX, TX                        int64
}

type radiusSession struct {
	CustomerID       string
	RX, TX           int64
	Started, Stopped bool
}

// BindRadiusSession checks the grant and credential version again at the point of authorization.
func (r *PortalRepository) BindRadiusSession(ctx context.Context, customer models.PortalCustomer, assignment models.PortalAssignment, sessionID string) (bool, error) {
	result := r.db.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Exec(`
		INSERT INTO portal_radius_sessions (node, session_id, customer_id)
		SELECT a.node, ?, c.id
		FROM portal_customers AS c
		JOIN portal_assignments AS a ON a.customer_id = c.id
		WHERE c.id = ? AND c.password_hash = ? AND NOT c.locked AND c.deleted_at IS NULL
		  AND c.policy_group_id IS NOT DISTINCT FROM CAST(? AS uuid)
          AND c.expires_at IS NOT DISTINCT FROM CAST(? AS timestamptz)
          AND (c.expires_at IS NULL OR c.expires_at > now())
		  AND (c.login_locked_until IS NULL OR c.login_locked_until <= now())
          AND c.traffic_type = ? AND c.traffic_size = ?
		  AND a.node = ? AND a.version = ? AND a.desired
		ON CONFLICT (node, session_id) DO UPDATE SET customer_id = EXCLUDED.customer_id
		WHERE portal_radius_sessions.customer_id = EXCLUDED.customer_id AND NOT portal_radius_sessions.stopped
	`, sessionID, customer.ID, customer.PasswordHash, customer.PolicyGroupID, customer.ExpiresAt, customer.TrafficType, customer.TrafficSize, assignment.Node, assignment.Version)
	return result.RowsAffected == 1, result.Error
}

// HasAuthorizedRadiusSession requires a password-authenticated session from the same node.
func (r *PortalRepository) HasAuthorizedRadiusSession(ctx context.Context, assignment models.PortalAssignment, packet RadiusAccounting) (bool, error) {
	var bound bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1 FROM portal_radius_sessions AS s
			JOIN portal_customers AS c ON c.id = s.customer_id
			JOIN portal_assignments AS a ON a.customer_id = c.id AND a.node = s.node
			WHERE s.node = ? AND s.session_id = ? AND NOT s.stopped
			  AND c.id = ? AND c.username = ? AND NOT c.locked AND c.deleted_at IS NULL
			  AND a.version = ? AND a.desired
		)
	`, packet.Node, packet.SessionID, assignment.CustomerID, packet.Username, assignment.Version).Scan(&bound).Error
	return bound, err
}

// RecordRadius credits increases in cumulative counters to the authenticated customer ID.
func (r *PortalRepository) RecordRadius(ctx context.Context, packet RadiusAccounting) (string, error) {
	var id string
	err := r.db.WithContext(ctx).Transaction(func(db *gorm.DB) error {
		// Keep the locking read's FROM clause out of the UPDATE.
		key := db.Table("portal_radius_sessions").
			Where("node = ? AND session_id = ?", packet.Node, packet.SessionID).
			Session(&gorm.Session{})
		var state radiusSession
		err := key.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&state).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		id = state.CustomerID
		if state.Stopped || (state.Started && packet.Event == models.EventHandshake) {
			return nil
		}
		rx, sent := max(packet.RX, state.RX), max(packet.TX, state.TX)
		if state.Started && packet.Event == models.EventPeriodicStats && rx == state.RX && sent == state.TX {
			return nil
		}
		err = key.Updates(map[string]any{
			"rx": rx, "tx": sent, "started": true, "stopped": packet.Event == models.EventDisconnect,
		}).Error
		if err != nil {
			return err
		}
		return db.Table("portal_radius_events").Create(map[string]any{
			"customer_id": id, "node": packet.Node, "created_at": packet.At, "event": packet.Event, "ip": packet.IP,
			"rx": rx - state.RX, "tx": sent - state.TX,
		}).Error
	})
	return id, err
}

func (r *PortalRepository) radiusEvents(ctx context.Context, id, node string, from, until *time.Time) *gorm.DB {
	query := r.db.WithContext(ctx).Table("portal_radius_events").Where("customer_id = ?", id)
	if node != "" {
		query = query.Where("node = ?", node)
	}
	if from != nil {
		query = query.Where("created_at >= ?", *from)
	}
	if until != nil {
		query = query.Where("created_at <= ?", *until)
	}
	return query
}

const radiusTotals = "COALESCE(SUM(rx),0) / 1073741824.0 AS rx, COALESCE(SUM(tx),0) / 1073741824.0 AS tx"

func (r *PortalRepository) RadiusBandwidth(ctx context.Context, id, node string, from, until *time.Time) (TotalBandwidths, error) {
	var total TotalBandwidths
	err := r.radiusEvents(ctx, id, node, from, until).Select(radiusTotals).Scan(&total).Error
	return total, err
}

func (r *PortalRepository) RadiusDaily(ctx context.Context, id string, from, until *time.Time) ([]models.DailyTraffic, error) {
	const day = "to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD')"
	var days []models.DailyTraffic
	err := r.radiusEvents(ctx, id, "", from, until).Select(day + " AS date, " + radiusTotals).Group(day).Scan(&days).Error
	return days, err
}

func (r *PortalRepository) RadiusActivity(ctx context.Context, id string, from, until *time.Time, offset, limit int) ([]models.OcservUserSessionLog, int64, error) {
	var total int64
	if err := r.radiusEvents(ctx, id, "", from, until).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []struct {
		Key string
		models.OcservUserSessionLog
	}
	err := r.radiusEvents(ctx, id, "", from, until).
		Select("id::text AS key, event, ip, created_at, node AS message").
		Order("created_at DESC, id DESC").Offset(offset).Limit(limit).Scan(&rows).Error
	logs := make([]models.OcservUserSessionLog, 0, len(rows))
	for _, row := range rows {
		row.PortalKey = row.Key
		logs = append(logs, row.OcservUserSessionLog)
	}
	return logs, total, err
}
