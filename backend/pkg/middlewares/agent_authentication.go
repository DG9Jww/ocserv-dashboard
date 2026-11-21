package middlewares

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/authz"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
)

type AgentCredentialStore interface {
	Get(context.Context) (*models.AgentToken, error)
}

type AgentOperatorStore interface {
	GetByUsername(context.Context, string) (*models.User, error)
}

// AgentAuthentication adds node-token support to the Agent's management routes.
// Usage: wrap the Agent's local session middleware with AgentAuthentication.
func AgentAuthentication(local echo.MiddlewareFunc, credentials AgentCredentialStore, users AgentOperatorStore, operator string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		localHandler := local(next)
		return func(c *echo.Context) error {
			token, ok := strings.CutPrefix(c.Request().Header.Get(echo.HeaderAuthorization), authz.AgentAuthorizationPrefix)
			if !ok {
				return localHandler(c)
			}
			nodePath, ok := strings.CutPrefix(c.Request().URL.Path, "/api")
			if !ok || !authz.AgentAPIPath(nodePath) || operator == "" || token == "" {
				return UnauthorizedError(c, "invalid agent authorization")
			}
			ctx := c.Request().Context()
			credential, err := credentials.Get(ctx)
			if err != nil || credential == nil || credential.Token == "" {
				return UnauthorizedError(c, "invalid agent authorization")
			}
			got, want := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(credential.Token))
			if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
				return UnauthorizedError(c, "invalid agent authorization")
			}
			user, err := users.GetByUsername(ctx, operator)
			if err != nil || user == nil || user.ID == 0 || !user.Superadmin {
				return UnauthorizedError(c, "agent operator is unavailable")
			}
			SetPrincipal(c, authz.Principal{UserID: user.ID, Username: user.Username, Superadmin: true})
			return next(c)
		}
	}
}
