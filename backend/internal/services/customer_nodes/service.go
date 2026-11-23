package customernodes

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/authz"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/usecase/customer_federation"
	"github.com/mmtaee/ocserv-dashboard/backend/pkg/middlewares"
	"gorm.io/gorm"
)

type Agent struct {
	Node   *federation.Node
	Tokens middlewares.AgentCredentialStore
}

func (a Agent) ServiceName() string { return "customer-node" }

func (a Agent) Register(api *echo.Group) {
	group := api.Group("/customer-node", a.authenticate)
	group.POST("/apply", func(c *echo.Context) error {
		var input federation.Apply
		if err := c.Bind(&input); err != nil {
			return failure(c, federation.ErrInvalid)
		}
		id, err := a.Node.Apply(c.Request().Context(), input)
		if err != nil {
			return failure(c, err)
		}
		return c.JSON(http.StatusOK, federation.NodeResult{OcservID: id, LifecycleApplied: true, PurgeApplied: input.Remove})
	})
	group.POST("/query", func(c *echo.Context) error {
		var input federation.Query
		if err := c.Bind(&input); err != nil {
			return failure(c, federation.ErrInvalid)
		}
		result, err := a.Node.Execute(c.Request().Context(), input)
		if err != nil {
			return failure(c, err)
		}
		return c.JSON(http.StatusOK, result)
	})
}

func (a Agent) authenticate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		req := c.Request()
		token, ok := strings.CutPrefix(req.Header.Get(echo.HeaderAuthorization), authz.AgentAuthorizationPrefix)
		if req.TLS == nil || !ok || token == "" {
			return middlewares.UnauthorizedError(c, "invalid agent authorization")
		}
		expected, err := a.Tokens.Get(req.Context())
		if err != nil || expected == nil || expected.Token == "" {
			return middlewares.UnauthorizedError(c, "invalid agent authorization")
		}
		got, want := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(expected.Token))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			return middlewares.UnauthorizedError(c, "invalid agent authorization")
		}
		return next(c)
	}
}

func RegisterAdmin(api *echo.Group, master *federation.Master, authenticate echo.MiddlewareFunc) {
	group := api.Group("/customer-management", authenticate, middlewares.SuperadminPermission())
	group.GET("", func(c *echo.Context) error {
		result, err := master.Inventory(c.Request().Context())
		if err != nil {
			return failure(c, err)
		}
		return c.JSON(http.StatusOK, result)
	})
	saveCustomer := func(c *echo.Context) error {
		var input federation.CustomerInput
		if err := c.Bind(&input); err != nil {
			return failure(c, federation.ErrInvalid)
		}
		return saved(c, master.SaveCustomer(c.Request().Context(), c.Param("id"), input))
	}
	group.POST("/accounts", saveCustomer)
	group.PATCH("/accounts/:id", saveCustomer)
	group.PATCH("/accounts/:id/status", func(c *echo.Context) error {
		var input struct {
			Locked *bool `json:"locked"`
		}
		if err := c.Bind(&input); err != nil || input.Locked == nil {
			return failure(c, federation.ErrInvalid)
		}
		return saved(c, master.SetCustomerStatus(c.Request().Context(), c.Param("id"), *input.Locked, false))
	})
	group.DELETE("/accounts/:id", func(c *echo.Context) error {
		return saved(c, master.SetCustomerStatus(c.Request().Context(), c.Param("id"), true, true))
	})

	saveGroup := func(c *echo.Context) error {
		var input models.PortalNodeGroup
		if err := c.Bind(&input); err != nil {
			return failure(c, federation.ErrInvalid)
		}
		input.ID = c.Param("id")
		return saved(c, master.SaveGroup(c.Request().Context(), input))
	}
	group.POST("/groups", saveGroup)
	group.PATCH("/groups/:id", saveGroup)
	group.PATCH("/nodes/:node", func(c *echo.Context) error {
		var input models.PortalNodeDefault
		if err := c.Bind(&input); err != nil {
			return failure(c, federation.ErrInvalid)
		}
		input.Node = c.Param("node")
		return saved(c, master.SaveDefaults(c.Request().Context(), input))
	})
}

func saved(c *echo.Context, err error) error {
	if err != nil {
		return failure(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func failure(c *echo.Context, err error) error {
	status, code := http.StatusServiceUnavailable, federation.ErrUnavailable.Error()
	switch {
	case errors.Is(err, federation.ErrInvalid), errors.Is(err, federation.ErrMissingDefaults), errors.Is(err, federation.ErrNoGrants),
		errors.Is(err, federation.ErrNodeSetup), errors.Is(err, federation.ErrNodeGroup),
		errors.Is(err, federation.ErrUsername), errors.Is(err, federation.ErrPassword):
		status, code = http.StatusBadRequest, err.Error()
	case errors.Is(err, federation.ErrConflict), errors.Is(err, gorm.ErrDuplicatedKey), strings.Contains(err.Error(), "SQLSTATE 23505"):
		status, code = http.StatusConflict, federation.ErrConflict.Error()
	case errors.Is(err, federation.ErrDefault), errors.Is(err, gorm.ErrRecordNotFound):
		status, code = http.StatusNotFound, federation.ErrDefault.Error()
	}
	return c.JSON(status, map[string]any{"message": []string{code}})
}
