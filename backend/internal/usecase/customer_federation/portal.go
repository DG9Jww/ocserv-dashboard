package federation

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	account "github.com/mmtaee/ocserv-dashboard/backend/internal/ocserv/user"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
	customerapi "github.com/mmtaee/ocserv-dashboard/backend/internal/usecase/customer_api"
	"github.com/mmtaee/ocserv-dashboard/backend/pkg/request"
	"golang.org/x/crypto/bcrypt"
)

type Availability struct {
	Partial     bool
	Unavailable bool
}

type availabilityKey struct{}

func WithAvailability(ctx context.Context) (context.Context, *Availability) {
	state := &Availability{}
	return context.WithValue(ctx, availabilityKey{}, state), state
}

func availability(ctx context.Context) *Availability {
	state, ok := ctx.Value(availabilityKey{}).(*Availability)
	if !ok {
		return &Availability{}
	}
	return state
}

type sessionClaims struct {
	ID      string `json:"sub"`
	Expires int64  `json:"exp"`
}

const jwtHeader = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"

func (m *Master) signature(body string) string {
	h := hmac.New(sha256.New, m.secret)
	h.Write([]byte("ocserv-unified-portal-v1\x00" + body))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func (m *Master) Login(ctx context.Context, input customerapi.Credentials) (*customerapi.LoginResponse, error) {
	customer, err := m.store.ByUsername(ctx, input.Username)
	if err != nil || customer.Locked || customer.DeletedAt != nil || bcrypt.CompareHashAndPassword(customer.PasswordHash, []byte(input.Password)) != nil {
		return nil, customerapi.ErrInvalidCredentials
	}
	expires := time.Now().Add(7 * time.Hour)
	payload, err := json.Marshal(sessionClaims{ID: customer.ID, Expires: expires.Unix()})
	if err != nil {
		return nil, err
	}
	body := jwtHeader + "." + base64.RawURLEncoding.EncodeToString(payload)
	return &customerapi.LoginResponse{User: customerapi.Customer{Username: customer.Username}, Token: body + "." + m.signature(body), ExpiresAt: expires}, nil
}

func (m *Master) AuthenticatePortalToken(ctx context.Context, token string) (string, error) {

	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != jwtHeader || !hmac.Equal([]byte(parts[2]), []byte(m.signature(parts[0]+"."+parts[1]))) {
		return "", customerapi.ErrInvalidToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", customerapi.ErrInvalidToken
	}
	var claims sessionClaims
	if json.Unmarshal(raw, &claims) != nil || !ValidID(claims.ID) || claims.Expires <= time.Now().Unix() {
		return "", customerapi.ErrInvalidToken
	}
	customer, err := m.store.Customer(ctx, claims.ID)
	if err != nil || customer.Locked || customer.DeletedAt != nil {
		return "", customerapi.ErrInvalidToken
	}
	return customer.Username, nil
}

func (m *Master) read(ctx context.Context, name string) (models.PortalCustomer, []models.PortalAssignment, error) {
	user, err := m.store.ByUsername(ctx, name)
	if err != nil {
		return user, nil, err
	}
	if user.Locked || user.DeletedAt != nil {
		return user, nil, customerapi.ErrInvalidToken
	}
	rows, err := m.store.Assignments(ctx, user.ID)
	return user, rows, err
}

func (m *Master) ChangePassword(ctx context.Context, name, password string) error {
	return m.store.Transaction(ctx, func(tx *repository.PortalRepository) error {
		user, err := tx.ByUsername(ctx, name)
		if err != nil {
			return err
		}
		if user.Locked || user.DeletedAt != nil {
			return customerapi.ErrInvalidToken
		}
		if err := setPassword(&user, password); err != nil {
			return err
		}
		return tx.UpdateCustomer(ctx, &user)
	})
}

func dateRange(input customerapi.DateRange) (from, until *time.Time, err error) {
	if input.DateStart != "" {
		t, e := time.Parse(time.DateOnly, input.DateStart)
		if e != nil {
			return nil, nil, ErrInvalid
		}
		from = &t
	}
	if input.DateEnd != "" {
		t, e := time.Parse(time.DateOnly, input.DateEnd)
		if e != nil {
			return nil, nil, ErrInvalid
		}
		t = t.AddDate(0, 0, 1).Add(-time.Nanosecond)
		until = &t
	}
	if from != nil && until != nil && from.After(*until) {
		return nil, nil, ErrInvalid
	}
	return
}

type nodeReply struct {
	node   models.PortalAssignment
	result NodeResult
	err    error
}

func (m *Master) gather(ctx context.Context, rows []models.PortalAssignment, q Query) ([]nodeReply, error) {
	out := make([]nodeReply, 0, len(rows))
	var pending sync.WaitGroup
	replies := make(chan nodeReply, len(rows))
	gate := make(chan struct{}, 8)
	for _, row := range rows {
		if !row.Desired {
			continue
		}
		pending.Go(func() {
			select {
			case gate <- struct{}{}:
				defer func() { <-gate }()
			case <-ctx.Done():
				replies <- nodeReply{node: row, err: ctx.Err()}
				return
			}
			result, err := m.query(ctx, row.Node, q)
			replies <- nodeReply{node: row, result: result, err: err}
		})
	}
	pending.Wait()
	close(replies)
	failures := 0
	for reply := range replies {
		if reply.err != nil {
			failures++
			continue
		}
		out = append(out, reply)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].node.Node < out[j].node.Node })
	if failures > 0 {
		availability(ctx).Partial = true
	}
	if failures > 0 && len(out) == 0 {
		return out, ErrUnavailable
	}
	return out, nil
}

func (m *Master) Summary(ctx context.Context, name string) (*customerapi.Summary, error) {
	user, _, err := m.read(ctx, name)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	out := &customerapi.Summary{OcservUser: customerapi.Customer{Username: name}, Usage: customerapi.Usage{DateStart: from, DateEnd: now}}
	out.OcservUser.TrafficType, out.OcservUser.TrafficSize = user.TrafficType, user.TrafficSize
	if user.ExpiresAt != nil {
		out.OcservUser.ExpiryMode, out.OcservUser.ExpireAt = models.ExpiryModeFixed, user.ExpiresAt
	}
	out.Usage.Bandwidths, err = m.store.RadiusBandwidth(ctx, user.ID, "", &from, &now)
	return out, err
}

func (m *Master) Bandwidth(ctx context.Context, name string, input customerapi.DateRange) (repository.TotalBandwidths, error) {
	from, until, err := dateRange(input)
	if err != nil {
		return repository.TotalBandwidths{}, err
	}
	user, _, err := m.read(ctx, name)
	if err != nil {
		return repository.TotalBandwidths{}, err
	}
	return m.store.RadiusBandwidth(ctx, user.ID, "", from, until)
}

func (m *Master) Statistics(ctx context.Context, name string, input customerapi.DateRange) ([]models.DailyTraffic, error) {
	from, until, err := dateRange(input)
	if err != nil {
		return nil, err
	}
	user, _, err := m.read(ctx, name)
	if err != nil {
		return nil, err
	}
	return m.store.RadiusDaily(ctx, user.ID, from, until)
}

func (m *Master) Sessions(ctx context.Context, name string) ([]models.OnlineUserSession, error) {
	_, rows, err := m.read(ctx, name)
	if err != nil {
		return nil, err
	}
	replies, err := m.gather(ctx, rows, Query{Operation: "sessions", RadiusUsername: name})
	out := make([]models.OnlineUserSession, 0)
	for _, reply := range replies {
		for _, session := range reply.result.Sessions {
			session.PortalKey = fmt.Sprintf("%s:%d", reply.node.Node, session.ID)
			out = append(out, session)
		}
	}
	return out, err
}

func (m *Master) action(ctx context.Context, name, operation string) error {
	_, rows, err := m.read(ctx, name)
	if err != nil {
		return err
	}
	_, err = m.gather(ctx, rows, Query{Operation: operation, RadiusUsername: name})
	return err
}

func (m *Master) Disconnect(ctx context.Context, name string) error {
	return m.action(ctx, name, "disconnect")
}
func (m *Master) Terminate(ctx context.Context, name string) error {
	return m.action(ctx, name, "terminate")
}

func (m *Master) Activities(ctx context.Context, name string, page *request.Pagination, input customerapi.DateRange) (*customerapi.Activities, error) {
	from, until, err := dateRange(input)
	if err != nil || page.Page < 1 || page.PageSize < 1 || page.PageSize > 100 || (page.Page-1) > int(^uint(0)>>1)/page.PageSize {
		return nil, ErrInvalid
	}
	user, _, err := m.read(ctx, name)
	if err != nil {
		return nil, err
	}
	logs, total, err := m.store.RadiusActivity(ctx, user.ID, from, until, (page.Page-1)*page.PageSize, page.PageSize)
	return &customerapi.Activities{Logs: &logs, Total: total}, err
}

func (m *Master) CiscoSetup(ctx context.Context, name string) (*customerapi.CiscoSetup, error) {
	user, rows, err := m.read(ctx, name)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.Node != user.DefaultNode || !row.Desired {
			continue
		}
		result, err := m.query(ctx, row.Node, Query{Operation: "profile", RadiusUsername: name})
		if err != nil || result.Profile == nil {
			return nil, ErrDefault
		}
		profile := result.Profile
		uri, err := account.BuildAnyConnectCreateURI(profile.Name, profile.Address, profile.Port, name)
		if err != nil {
			return nil, err
		}
		return &customerapi.CiscoSetup{ConnectionCreateURI: uri, Radius: true, ConnectionName: profile.Name, ServerAddress: profile.Address, ServerPort: profile.Port}, nil
	}
	return nil, ErrDefault
}
