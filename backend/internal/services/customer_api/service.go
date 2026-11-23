package customerapi

import (
	"github.com/labstack/echo/v5"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/usecase/customer_federation"
	"github.com/mmtaee/ocserv-dashboard/backend/pkg/middlewares"
)

type Service struct {
	controller   *CustomerController
	authenticate echo.MiddlewareFunc
}

func (s *Service) ServiceName() string { return "customer-api" }

func NewFederated(master *federation.Master) *Service {
	controller := newCustomerController(master)
	return &Service{controller: controller, authenticate: func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			token, err := middlewares.BearerToken(c)
			if err != nil {
				return middlewares.UnauthorizedError(c, "missing or invalid Authorization header")
			}
			name, err := master.AuthenticatePortalToken(c.Request().Context(), token)
			if err != nil {
				return middlewares.UnauthorizedError(c, "invalid token")
			}
			c.Set("unified-customer-name", name)
			ctx, state := federation.WithAvailability(c.Request().Context())
			c.SetRequest(c.Request().WithContext(ctx))
			response, err := echo.UnwrapResponse(c.Response())
			if err != nil {
				return err
			}
			response.Before(func() {
				if state.Partial {
					response.Header().Set("X-Ocserv-Partial", "true")
				}
				if state.Unavailable {
					response.Header().Set("X-Ocserv-Unavailable", "true")
				}
			})
			response.Header().Set("Cache-Control", "no-store")
			return next(c)
		}
	}}
}
