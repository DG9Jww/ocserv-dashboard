package units

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	controller "github.com/mmtaee/ocserv-dashboard/backend/internal/services/admin_api/ocserv_agent"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/usecase/admin_api/agents"
	"github.com/mmtaee/ocserv-dashboard/backend/pkg/middlewares"
	"github.com/stretchr/testify/require"
)

type agentProxyTransport func(*http.Request) (*http.Response, error)

func (roundTrip agentProxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return roundTrip(req)
}

func TestAgentProxyAuthenticatesBothHops(t *testing.T) {
	ctx := context.Background()
	agentUC := agents.New(newAgentRepository())
	input := agents.CreateInput{
		Name: "edge", AddressType: models.AgentAddressTypeDomain, Address: "edge.example.com", Token: "node-secret",
	}
	node, err := agentUC.Create(ctx, input)
	require.NoError(t, err)
	tokenStore := &localAgentTokenRepository{current: &models.AgentToken{ID: 1, Token: input.Token}}
	userStore := loginUserRepository{user: &models.User{ID: 42, Username: "nodeadmin", Superadmin: true}}
	local := middlewares.AuthMiddleware(staticAuthenticator{})
	agent := echo.New()
	agent.PATCH("/api/system/ocserv-config", func(c *echo.Context) error {
		actor, err := middlewares.Principal(c)
		require.NoError(t, err)
		require.Equal(t, uint(42), actor.UserID)
		require.Equal(t, "yes", c.QueryParam("check"))
		payload := map[string]string{}
		require.NoError(t, json.NewDecoder(c.Request().Body).Decode(&payload))
		require.Equal(t, "hello", payload["banner"])
		c.Response().Header().Set("Set-Cookie", "example=unexpected")
		return c.JSON(http.StatusOK, payload)
	}, middlewares.AgentAuthentication(local, tokenStore, userStore, "nodeadmin"))

	calls := 0
	proxy, err := agents.NewProxy(agentUC, "https", agentProxyTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "https", req.URL.Scheme)
		require.Equal(t, "edge.example.com:8080", req.URL.Host)
		require.Equal(t, "OcservAgent node-secret", req.Header.Get("Authorization"))
		require.Empty(t, req.Header.Get("Cookie"))
		out := httptest.NewRecorder()
		agent.ServeHTTP(out, req)
		return out.Result(), nil
	}))
	require.NoError(t, err)
	ctl := controller.New(agentUC, proxy)
	master := echo.New()
	adminRoutes := master.Group("/api/ocserv/agents", local, middlewares.SuperadminPermission())
	adminRoutes.PATCH("/:id/proxy/*", ctl.Proxy)
	adminRoutes.GET("", ctl.List)
	requestPath := fmt.Sprintf("/api/ocserv/agents/%d/proxy", node.ID)
	run := func(method, path, credential string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(`{"banner":"hello"}`))
		req.Header.Set("Authorization", credential)
		req.Header.Set("Cookie", "browser=private")
		out := httptest.NewRecorder()
		master.ServeHTTP(out, req)
		return out
	}

	for _, test := range []struct {
		path, credential string
		status           int
	}{
		{"/system/ocserv-config", "", http.StatusUnauthorized},
		{"/system/ocserv-config", "Bearer normal", http.StatusForbidden},
		{"/auth/logout", "Bearer superadmin", http.StatusNotFound},
		{"/ocserv/users/../..", "Bearer superadmin", http.StatusNotFound},
	} {
		require.Equal(t, test.status, run(http.MethodPatch, requestPath+test.path, test.credential).Code)
	}
	require.Zero(t, calls)
	ok := run(http.MethodPatch, requestPath+"/system/ocserv-config?check=yes", "Bearer superadmin")
	require.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
	require.JSONEq(t, `{"banner":"hello"}`, ok.Body.String())
	require.Empty(t, ok.Header().Get("Set-Cookie"))

	list := run(http.MethodGet, "/api/ocserv/agents", "Bearer superadmin")
	require.Equal(t, http.StatusOK, list.Code)
	require.NotContains(t, list.Body.String(), input.Token)

	tokenStore.current.Token = "rotated"
	rejected := run(http.MethodPatch, requestPath+"/system/ocserv-config?check=yes", "Bearer superadmin")
	require.Equal(t, http.StatusBadGateway, rejected.Code)
	require.NotContains(t, rejected.Body.String(), input.Token)
}

func TestAgentEditingKeepsCredentialOnlyForSameDestination(t *testing.T) {
	ctx := context.Background()
	agentUC := agents.New(newAgentRepository())
	agent, err := agentUC.Create(ctx, agents.CreateInput{
		Name: "edge", AddressType: models.AgentAddressTypeDomain, Address: "edge.example.com", Token: "node-secret",
	})
	require.NoError(t, err)
	edit := agents.UpdateInput{Name: "renamed", AddressType: agent.AddressType, Address: agent.Address, Port: agent.Port}
	updated, err := agentUC.Update(ctx, agent.ID, edit)
	require.NoError(t, err)
	require.Equal(t, "node-secret", updated.Token)
	edit.Address = "another.example.com"
	_, err = agentUC.Update(ctx, agent.ID, edit)
	require.ErrorIs(t, err, agents.ErrAgentTokenRequired)
}

func TestAgentHealthUsesAuthenticatedSystemEndpointAndCachesResults(t *testing.T) {
	ctx := context.Background()
	usecase := agents.New(newAgentRepository())
	nodes := make(map[string]*models.OcservAgent)
	for _, name := range []string{"online", "offline", "denied", "tls", "invalid"} {
		node, err := usecase.Create(ctx, agents.CreateInput{
			Name: name, AddressType: models.AgentAddressTypeDomain,
			Address: name + ".example.com", Token: "token-" + name,
		})
		require.NoError(t, err)
		nodes[name] = node
	}

	var calls atomic.Int32
	proxy, err := agents.NewProxy(usecase, "https", agentProxyTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		name, _, _ := strings.Cut(req.URL.Hostname(), ".")
		if req.URL.Scheme != "https" || req.URL.Port() != "8080" || req.URL.Path != "/api/system" || req.Method != http.MethodGet {
			t.Errorf("unexpected probe destination: %s %s", req.Method, req.URL)
		}
		if req.Header.Get("Authorization") != "OcservAgent token-"+name {
			return &http.Response{StatusCode: http.StatusUnauthorized, Body: http.NoBody}, nil
		}
		deadline, ok := req.Context().Deadline()
		if !ok || time.Until(deadline) > 3*time.Second {
			t.Error("probe has no short deadline")
		}
		switch name {
		case "offline":
			return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
		case "denied":
			return &http.Response{StatusCode: http.StatusForbidden, Body: http.NoBody}, nil
		case "tls":
			return nil, &tls.CertificateVerificationError{Err: errors.New("untrusted certificate")}
		case "invalid":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":"Healthy"}`))}, nil
		default:
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"first_init":false,"google_captcha_secret_key":"private-settings"}`))}, nil
		}
	}))
	require.NoError(t, err)

	master := echo.New()
	admin := master.Group("/api/ocserv/agents", middlewares.AuthMiddleware(staticAuthenticator{}), middlewares.SuperadminPermission())
	admin.GET("/status", controller.New(usecase, proxy).Health)
	getStatus := func(credential string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/ocserv/agents/status", nil)
		req.Header.Set("Authorization", credential)
		out := httptest.NewRecorder()
		master.ServeHTTP(out, req)
		return out
	}
	require.Equal(t, http.StatusUnauthorized, getStatus("").Code)
	require.Equal(t, http.StatusForbidden, getStatus("Bearer normal").Code)
	require.Zero(t, calls.Load())

	read := func() agents.HealthSnapshot {
		out := getStatus("Bearer superadmin")
		require.Equal(t, http.StatusOK, out.Code)
		require.Equal(t, "no-store", out.Header().Get("Cache-Control"))
		require.NotContains(t, out.Body.String(), "private-settings")
		require.NotContains(t, out.Body.String(), "token-")
		var snapshot agents.HealthSnapshot
		require.NoError(t, json.Unmarshal(out.Body.Bytes(), &snapshot))
		return snapshot
	}
	started := read()
	require.True(t, started.Refreshing)
	require.Equal(t, agents.HealthChecking, started.Statuses[nodes["online"].ID])

	waitForRefresh := func() agents.HealthSnapshot {
		deadline := time.Now().Add(time.Second)
		for {
			snapshot := read()
			if !snapshot.Refreshing {
				return snapshot
			}
			require.False(t, time.Now().After(deadline), "probe batch did not finish")
			time.Sleep(time.Millisecond)
		}
	}
	snapshot := waitForRefresh()
	require.Equal(t, agents.HealthOnline, snapshot.Statuses[nodes["online"].ID])
	require.Equal(t, agents.HealthOffline, snapshot.Statuses[nodes["offline"].ID])
	for _, name := range []string{"denied", "tls", "invalid"} {
		require.Equal(t, agents.HealthUnavailable, snapshot.Statuses[nodes[name].ID], name)
	}
	require.Equal(t, snapshot, read())
	require.EqualValues(t, len(nodes), calls.Load())

	node := nodes["online"]
	_, err = usecase.Update(ctx, node.ID, agents.UpdateInput{
		Name: node.Name, Address: node.Address, AddressType: node.AddressType, Port: node.Port, Token: "rotated",
	})
	require.NoError(t, err)
	require.Equal(t, agents.HealthChecking, read().Statuses[node.ID])
	require.Equal(t, agents.HealthUnavailable, waitForRefresh().Statuses[node.ID])
	require.EqualValues(t, len(nodes)+1, calls.Load())
}
