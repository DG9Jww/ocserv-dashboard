package federation

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
	Pending     bool
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
		if err := m.setPassword(&user, password); err != nil {
			return err
		}
		if err := tx.Save(ctx, &user); err != nil {
			return err
		}
		groups, err := tx.Groups(ctx)
		if err != nil {
			return err
		}
		availability(ctx).Pending = len(effective(user, groups)) > 0
		return synchronize(ctx, tx, user, groups, "*")
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

func (m *Master) gather(ctx context.Context, rows []models.PortalAssignment, q Query, live bool) ([]nodeReply, error) {
	out := make([]nodeReply, 0, len(rows))
	var pending sync.WaitGroup
	replies := make(chan nodeReply, len(rows))
	gate := make(chan struct{}, 8)
	skipped := 0
	for _, row := range rows {
		if live && !row.Desired {
			continue
		}
		if row.OcservID == 0 {
			if row.Desired {
				skipped++
			}
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
			result, err := m.query(ctx, row, q)
			replies <- nodeReply{node: row, result: result, err: err}
		})
	}
	pending.Wait()
	close(replies)
	failures := skipped
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
	user, rows, err := m.read(ctx, name)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	out := &customerapi.Summary{OcservUser: customerapi.Customer{Username: name}, Usage: customerapi.Usage{DateStart: from, DateEnd: now}}
	for _, row := range rows {
		if row.Desired && row.Applied != row.Version {
			availability(ctx).Pending = true
		}
		if row.Node != user.DefaultNode || !row.Desired || row.OcservID == 0 || row.Applied != row.Version {
			continue
		}
		result, err := m.query(ctx, row, Query{Operation: "account"})
		if err != nil || result.Account == nil {
			availability(ctx).Partial = true
			continue
		}
		a := result.Account
		out.OcservUser = customerapi.Customer{Username: name, Owner: a.Owner.Username, IsLocked: a.IsLocked, ExpiryMode: a.ExpiryMode, ExpireAt: a.ExpireAt,
			ExpireDaysAfterFirstConnection: a.ExpireDaysAfterFirstConnection, FirstConnectedAt: a.FirstConnectedAt, DeactivatedAt: a.DeactivatedAt,
			TrafficType: a.TrafficType, TrafficSize: a.TrafficSize, RunningRx: a.RunningRx, RunningTx: a.RunningTx}
	}
	replies, err := m.gather(ctx, rows, Query{Operation: "bandwidth", From: &from, Until: &now}, false)
	if errors.Is(err, ErrUnavailable) {
		availability(ctx).Unavailable = true
	} else if err != nil {
		return nil, err
	}
	out.Usage.Bandwidths = sumBandwidth(replies)
	return out, nil
}

func sumBandwidth(replies []nodeReply) repository.TotalBandwidths {
	var out repository.TotalBandwidths
	for _, reply := range replies {
		out.RX += reply.result.Bandwidth.RX
		out.TX += reply.result.Bandwidth.TX
	}
	return out
}

func (m *Master) Bandwidth(ctx context.Context, name string, input customerapi.DateRange) (repository.TotalBandwidths, error) {
	from, until, err := dateRange(input)
	if err != nil {
		return repository.TotalBandwidths{}, err
	}
	_, rows, err := m.read(ctx, name)
	if err != nil {
		return repository.TotalBandwidths{}, err
	}
	replies, err := m.gather(ctx, rows, Query{Operation: "bandwidth", From: from, Until: until}, false)
	return sumBandwidth(replies), err
}

func (m *Master) Statistics(ctx context.Context, name string, input customerapi.DateRange) ([]models.DailyTraffic, error) {
	from, until, err := dateRange(input)
	if err != nil {
		return nil, err
	}
	_, rows, err := m.read(ctx, name)
	if err != nil {
		return nil, err
	}
	replies, err := m.gather(ctx, rows, Query{Operation: "daily", From: from, Until: until}, false)
	if err != nil {
		return nil, err
	}
	byDay := make(map[string]models.DailyTraffic)
	for _, reply := range replies {
		for _, day := range reply.result.Daily {
			sum := byDay[day.Date]
			sum.Date, sum.Rx, sum.Tx = day.Date, sum.Rx+day.Rx, sum.Tx+day.Tx
			byDay[day.Date] = sum
		}
	}
	out := make([]models.DailyTraffic, 0, len(byDay))
	for _, day := range byDay {
		out = append(out, day)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}

func (m *Master) Sessions(ctx context.Context, name string) ([]models.OnlineUserSession, error) {
	_, rows, err := m.read(ctx, name)
	if err != nil {
		return nil, err
	}
	replies, err := m.gather(ctx, rows, Query{Operation: "sessions"}, true)
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
	_, err = m.gather(ctx, rows, Query{Operation: operation}, true)
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
	_, rows, err := m.read(ctx, name)
	if err != nil {
		return nil, err
	}
	query := Query{Operation: "activity", From: from, Until: until, Limit: page.PageSize}
	replies, err := m.gather(ctx, rows, query, false)
	if err != nil {
		return nil, err
	}
	for {
		result, failed, err := m.mergeActivity(ctx, slices.Clone(replies), query, page)
		if err == nil {
			return result, nil
		}
		availability(ctx).Partial = true
		replies = slices.DeleteFunc(replies, func(reply nodeReply) bool { return reply.node.Node == failed })
		if ctx.Err() != nil || len(replies) == 0 {
			return nil, ErrUnavailable
		}
	}
}

func (m *Master) mergeActivity(ctx context.Context, replies []nodeReply, query Query, page *request.Pagination) (*customerapi.Activities, string, error) {
	logs := make([]models.OcservUserSessionLog, 0, page.PageSize)
	positions := make([]int, len(replies))
	offsets := make([]int, len(replies))
	var total int64
	for _, reply := range replies {
		total += reply.result.Total
	}
	skip := (page.Page - 1) * page.PageSize
	for int64(skip) < total && len(logs) < page.PageSize {
		next := -1
		for i, reply := range replies {
			if positions[i] == len(reply.result.Activities) {
				continue
			}
			if next < 0 || reply.result.Activities[positions[i]].CreatedAt.After(replies[next].result.Activities[positions[next]].CreatedAt) {
				next = i
			}
		}
		if next < 0 {
			break
		}
		stream := &replies[next]
		row := stream.result.Activities[positions[next]]
		row.PortalKey = stream.node.Node + ":" + row.PortalKey
		positions[next]++
		if skip == 0 {
			logs = append(logs, row)
		} else {
			skip--
		}
		if len(logs) == page.PageSize || positions[next] < len(stream.result.Activities) {
			continue
		}
		offsets[next] += positions[next]
		if int64(offsets[next]) >= stream.result.Total {
			continue
		}
		query.Offset = offsets[next]
		part, err := m.query(ctx, stream.node, query)
		if err != nil {
			return nil, stream.node.Node, err
		}
		stream.result.Activities, positions[next] = part.Activities, 0
	}
	return &customerapi.Activities{Logs: &logs, Total: total}, "", nil
}

func (m *Master) defaultConnection(ctx context.Context, name string) (models.PortalCustomer, models.PortalNodeDefault, *NodeProfile, error) {
	user, rows, err := m.read(ctx, name)
	if err != nil {
		return user, models.PortalNodeDefault{}, nil, err
	}
	for _, row := range rows {
		if row.Node != user.DefaultNode || !row.Desired {
			continue
		}
		if row.Applied != row.Version || row.OcservID == 0 {
			return user, models.PortalNodeDefault{}, nil, ErrPending
		}
		defaults, err := m.store.Defaults(ctx)
		if err != nil {
			return user, models.PortalNodeDefault{}, nil, err
		}
		d, err := defaultsFor(defaults, row.Node)
		if err != nil {
			return user, d, nil, ErrDefault
		}
		profile, err := m.query(ctx, row, Query{Operation: "profile"})
		if err != nil || profile.Profile == nil {
			return user, d, nil, ErrDefault
		}
		return user, d, profile.Profile, nil
	}
	return user, models.PortalNodeDefault{}, nil, ErrDefault
}

func (m *Master) CiscoSetup(ctx context.Context, name, _ string) (*customerapi.CiscoSetup, error) {
	user, _, profile, err := m.defaultConnection(ctx, name)
	if err != nil {
		return nil, err
	}
	password, err := m.decrypt(user.PasswordBox, "password:"+name)
	if err != nil {
		return nil, err
	}
	uri, err := account.BuildAnyConnectCreateURI(profile.Name, profile.Address, profile.Port, name)
	if err != nil {
		return nil, err
	}
	return &customerapi.CiscoSetup{ConnectionCreateURI: uri, CertificatePassword: string(password), ConnectionName: profile.Name, ServerAddress: profile.Address, ServerPort: profile.Port}, nil
}

func (m *Master) Download(ctx context.Context, name string) ([]byte, error) {
	user, d, _, err := m.defaultConnection(ctx, name)
	if err != nil {
		return nil, err
	}
	return m.PKCS12(ctx, user, scope(d))
}
