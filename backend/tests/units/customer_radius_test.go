package units

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/services/customer_nodes"
	federation "github.com/mmtaee/ocserv-dashboard/backend/internal/usecase/customer_federation"
	"github.com/stretchr/testify/require"
)

type radiusStub struct {
	authorization [4]string
	policy        federation.RadiusPolicy
	err           error
	packet        repository.RadiusAccounting
}

func (s *radiusStub) AuthorizeRadius(_ context.Context, node, username, password, sessionID string) (federation.RadiusPolicy, error) {
	s.authorization = [4]string{node, username, password, sessionID}
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
	  "Acct-Session-Id":{"type":"string","value":["session"]}}
	`
	stub := &radiusStub{policy: federation.RadiusPolicy{Group: "sales", SessionTimeout: 60}}
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
	require.JSONEq(t, `{"reply:Class":"OU=sales","reply:Session-Timeout":60}`, out.Body.String())
	require.Equal(t, [4]string{"agent-2", "customer", "example-password", "session"}, stub.authorization)
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
