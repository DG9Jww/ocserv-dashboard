package ocserv_agent

import (
	"errors"
	"io"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/usecase/admin_api/agents"
	"gorm.io/gorm"
)

// Proxy forwards a registered node operation for an authenticated superadmin.
func (ctl *Controller) Proxy(c *echo.Context) error {
	id, err := parseID(c.Param("id"))
	if err != nil {
		return ctl.request.BadRequest(c, err)
	}
	response, err := ctl.proxy.Forward(id, "/"+c.Param("*"), c.Request())
	switch {
	case errors.Is(err, agents.ErrAgentPath), errors.Is(err, gorm.ErrRecordNotFound):
		return echo.NewHTTPError(http.StatusNotFound, "agent or agent API path not found")
	case err != nil:
		return echo.NewHTTPError(http.StatusBadGateway, "unable to reach the agent API")
	}
	defer response.Body.Close()

	status := response.StatusCode
	if status >= 300 && status < 400 {
		return echo.NewHTTPError(http.StatusBadGateway, "unexpected redirect from the agent API")
	}
	if status == http.StatusUnauthorized {
		return echo.NewHTTPError(http.StatusBadGateway, "agent rejected its registered credential")
	}
	out := c.Response()
	for _, header := range []string{"Content-Type", "Content-Encoding", "Content-Disposition"} {
		for _, value := range response.Header.Values(header) {
			out.Header().Add(header, value)
		}
	}
	out.Header().Set("Cache-Control", "no-store")
	out.Header().Set("X-Content-Type-Options", "nosniff")
	out.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	out.WriteHeader(status)
	_, err = io.Copy(out, response.Body)
	return err
}
