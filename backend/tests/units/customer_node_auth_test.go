package units

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	customernodes "github.com/mmtaee/ocserv-dashboard/backend/internal/services/customer_nodes"
	"github.com/stretchr/testify/require"
)

type portalTestToken struct{}

func (portalTestToken) Get(context.Context) (*models.AgentToken, error) {
	return &models.AgentToken{Token: "test-node-token"}, nil
}

func TestCustomerNodeEndpointsRequireTLSAndNodeToken(t *testing.T) {
	server := echo.New()
	customernodes.Agent{Tokens: portalTestToken{}}.Register(server.Group("/api"))
	cases := []struct {
		name, auth string
		tls        bool
		status     int
	}{
		{"plaintext", "OcservAgent test-node-token", false, http.StatusUnauthorized},
		{"portal bearer", "Bearer test-node-token", true, http.StatusUnauthorized},
		{"wrong token", "OcservAgent wrong", true, http.StatusUnauthorized},
		{"valid token reaches request validation", "OcservAgent test-node-token", true, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/customer-node/query", strings.NewReader("{"))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			req.Header.Set(echo.HeaderAuthorization, tc.auth)
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)
			require.Equal(t, tc.status, rec.Code)
		})
	}
}
