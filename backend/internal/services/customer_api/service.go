package customerapi

import (
	"github.com/labstack/echo/v5"
	"github.com/mmtaee/ocserv-dashboard/backend/config"
	ocservaccount "github.com/mmtaee/ocserv-dashboard/backend/internal/ocserv/user"
	platformocserv "github.com/mmtaee/ocserv-dashboard/backend/internal/platform/ocserv"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
	customerusecase "github.com/mmtaee/ocserv-dashboard/backend/internal/usecase/customer_api"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/usecase/customer_federation"
	"github.com/mmtaee/ocserv-dashboard/backend/pkg/middlewares"
)

type Service struct {
	controller   *CustomerController
	authenticate echo.MiddlewareFunc
}

func (s *Service) ServiceName() string { return "customer-api" }

// New constructs the Customer API dependency graph.
func New(cfg *config.Config) *Service {
	usecase := customerusecase.New(
		repository.NewSystemRepository(),
		repository.NewtOcservUserRepository(),
		platformocserv.NewClient(),
		cfg.SecretKey,
		ocservaccount.NewOcservUser(),
	)
	return &Service{controller: newCustomerController(usecase), authenticate: AuthMiddleware(usecase)}
}

// AuthMiddleware authenticates customer JWT bearer tokens.
// Usage: e.Use(customerapi.AuthMiddleware(usecase))
func AuthMiddleware(usecase *customerusecase.Usecase) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			token, err := middlewares.BearerToken(c)
			if err != nil {
				return middlewares.UnauthorizedError(c, "missing or invalid Authorization header")
			}
			principal, err := usecase.AuthenticateToken(c.Request().Context(), token)
			if err != nil {
				return middlewares.UnauthorizedError(c, "invalid token")
			}
			middlewares.SetPrincipal(c, principal)
			return next(c)
		}
	}
}

func NewFederated(master *federation.Master) *Service {
	controller := newCustomerController(master)
	controller.download = master.Download
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
				if state.Pending {
					response.Header().Set("X-Ocserv-Sync-Pending", "true")
				}
			})
			response.Header().Set("Cache-Control", "no-store")
			response.Header().Set("X-Ocserv-Managed", "true")
			return next(c)
		}
	}}
}
