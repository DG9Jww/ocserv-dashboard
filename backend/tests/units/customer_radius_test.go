package units

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/platform/database"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/services/customer_nodes"
	federation "github.com/mmtaee/ocserv-dashboard/backend/internal/usecase/customer_federation"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type radiusStub struct {
	authorization [5]string
	policy        federation.RadiusPolicy
	err           error
	packet        repository.RadiusAccounting
}

func TestRecordRadiusStartUpdatesLockedSession(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	originalDB := database.PostgresDB
	database.PostgresDB = db
	t.Cleanup(func() { database.PostgresDB = originalDB })

	const customerID = "018f5485-0000-7000-8000-000000000001"
	packet := repository.RadiusAccounting{
		Node: "master", SessionID: "session", Event: models.EventHandshake,
		At: time.Now().UTC(), IP: "192.0.2.1",
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`^SELECT \* FROM "portal_radius_sessions" WHERE node = \$1 AND session_id = \$2 LIMIT \$3 FOR UPDATE$`).
		WithArgs(packet.Node, packet.SessionID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"customer_id", "rx", "tx", "started", "stopped"}).AddRow(customerID, 0, 0, false, false))
	mock.ExpectExec(`^UPDATE "portal_radius_sessions" SET "last_seen_at"=GREATEST\(last_seen_at, \$1\),"rx"=\$2,"started"=\$3,"stopped"=\$4,"tx"=\$5 WHERE node = \$6 AND session_id = \$7$`).
		WithArgs(packet.At, 0, true, false, 0, packet.Node, packet.SessionID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`^INSERT INTO "portal_radius_events"`).
		WithArgs(packet.At, customerID, packet.Event, packet.IP, packet.Node, 0, 0).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	id, err := repository.NewPortalRepository().RecordRadius(context.Background(), packet)
	require.NoError(t, err)
	require.Equal(t, customerID, id)
	require.NoError(t, mock.ExpectationsWereMet())
}

func (s *radiusStub) AuthorizeRadius(_ context.Context, node, username, password, sessionID, packetVector string) (federation.RadiusPolicy, error) {
	s.authorization = [5]string{node, username, password, sessionID, packetVector}
	return s.policy, s.err
}

func (s *radiusStub) AccountRadius(_ context.Context, packet repository.RadiusAccounting) error {
	s.packet = packet
	return s.err
}

func TestRadiusCallbackAuthorize(t *testing.T) {
	const body = `{
	  "NAS-Identifier":{"type":"string","value":["agent-2"]},
	  "User-Name":{"type":"string","value":["customer"]},
	  "User-Password":{"type":"string","value":["example-password"]},
	  "Acct-Session-Id":{"type":"string","value":["session"]},
	  "Tmp-String-0":{"type":"string","value":["0x0123456789abcdef0123456789abcdef"]}}
	`
	stub := &radiusStub{policy: federation.RadiusPolicy{SessionTimeout: 60}}
	const secret = "testing-only-callback-secret-value"
	handler := customernodes.RadiusHandler(stub, secret)

	request := func(key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/authorize", strings.NewReader(body))
		req.SetBasicAuth("ocserv-dashboard", key)
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		return out
	}

	require.Equal(t, http.StatusUnauthorized, request("bad").Code)
	require.Empty(t, stub.authorization[0])
	out := request(secret)
	require.Equal(t, http.StatusOK, out.Code)
	require.JSONEq(t, `{"reply:Session-Timeout":60}`, out.Body.String())
	require.Equal(t, [5]string{"agent-2", "customer", "example-password", "session", "0x0123456789abcdef0123456789abcdef"}, stub.authorization)
	stub.policy = federation.RadiusPolicy{}
	require.Equal(t, http.StatusNoContent, request(secret).Code)
	stub.policy.Network = models.PortalPolicyGroup{
		PrimaryDNS: "10.1.0.53", SecondaryDNS: "10.1.0.54",
		Routes: []string{"10.1.0.0/24", "192.168.10.0/24"}, UploadKBPerSecond: 2000, DownloadKBPerSecond: 8000, MaxConnections: 2,
	}
	out = request(secret)
	require.Equal(t, http.StatusOK, out.Code)
	require.JSONEq(t, `{
		"reply:MS-Primary-DNS-Server":"10.1.0.53",
		"reply:MS-Secondary-DNS-Server":"10.1.0.54",
		"reply:Framed-Route":{"op":"+=","value":["10.1.0.0/24","192.168.10.0/24"]},
		"reply:RP-Upstream-Speed-Limit":2000,
		"reply:Acct-Interim-Interval":60,
		"reply:RP-Downstream-Speed-Limit":8000
	}`, out.Body.String())
	stub.err = federation.ErrRadiusRejected
	require.Equal(t, http.StatusUnauthorized, request(secret).Code)
}

func TestRadiusCallbackAccountingUsesOcservCounterDirection(t *testing.T) {
	const secret = "testing-only-callback-secret-value"
	const body = `{
	  "NAS-Identifier":{"value":["agent-2"]},
	  "User-Name":{"value":["customer"]},
	  "Acct-Session-Id":{"value":["session"]},
	  "Acct-Status-Type":{"value":["Interim-Update"]},
	  "Acct-Input-Octets":{"value":[10]},
	  "Acct-Input-Gigawords":{"value":["1"]},
	  "Acct-Output-Octets":{"value":[42]},
	  "Calling-Station-Id":{"value":["2001:db8::2"]}}
	`
	stub := &radiusStub{}
	req := httptest.NewRequest(http.MethodPost, "/accounting", strings.NewReader(body))
	req.SetBasicAuth("ocserv-dashboard", secret)
	out := httptest.NewRecorder()
	customernodes.RadiusHandler(stub, secret).ServeHTTP(out, req)
	require.Equal(t, http.StatusNoContent, out.Code)
	require.Equal(t, models.EventPeriodicStats, stub.packet.Event)
	require.Equal(t, int64(1<<32+10), stub.packet.RX)
	require.Equal(t, int64(42), stub.packet.TX)
	require.Equal(t, "session", stub.packet.SessionID)
	require.Equal(t, "2001:db8::2", stub.packet.IP)
}
