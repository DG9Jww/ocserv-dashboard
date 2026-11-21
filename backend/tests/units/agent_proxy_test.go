package units

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
