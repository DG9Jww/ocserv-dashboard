package units

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/mmtaee/ocserv-dashboard/backend/config"
	adminapi "github.com/mmtaee/ocserv-dashboard/backend/internal/services/admin_api"
	"github.com/stretchr/testify/require"
)

func TestServiceFlagDefaultsAndBooleanParsing(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_ENABLED", "")
	t.Setenv("CUSTOMER_API_ENABLED", "")
	config.Init(false, "", 0)
	require.False(t, config.Get().TelegramEnabled)
	require.True(t, config.Get().CustomerAPIEnabled)

	t.Setenv("TELEGRAM_BOT_ENABLED", "1")
	t.Setenv("CUSTOMER_API_ENABLED", "false")
	config.Init(false, "", 0)
	require.True(t, config.Get().TelegramEnabled)
	require.False(t, config.Get().CustomerAPIEnabled)

	t.Setenv("TELEGRAM_BOT_ENABLED", "")
	t.Setenv("CUSTOMER_API_ENABLED", "")
	config.Init(false, "", 0)
}

func TestConfigValidateRequiresStrongSecretKey(t *testing.T) {
	for _, secretKey := range []string{"", "too-short"} {
		t.Setenv("SECRET_KEY", secretKey)
		config.Init(false, "", 0)
		require.Error(t, config.Validate())
	}

	t.Setenv("SECRET_KEY", "0123456789abcdef0123456789abcdef")
	config.Init(false, "", 0)
	require.NoError(t, config.Validate())
}

func TestTelegramRoutesFollowServiceFlag(t *testing.T) {
	t.Setenv("AGENT_NODE", "true")
	config.Init(false, "", 0)
	for _, enabled := range []bool{false, true} {
		service, err := adminapi.New(enabled, false)
		require.NoError(t, err)
		e := echo.New()
		service.Register(e.Group(""))
		found := false
		for _, route := range e.Router().Routes() {
			found = found || route.Path == "/telegram/settings"
		}
		require.Equal(t, enabled, found)
	}
}

func TestOcservAuthConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ocserv.conf")
	for _, tc := range []struct {
		name, file                   string
		radius, certificate, invalid bool
	}{
		{name: "certificate", file: "auth = \"certificate\"", certificate: true},
		{name: "radius", file: "# auth = certificate\nauth = \"radius[config=/etc/radcli/radiusclient.conf,groupconfig=true]\" # managed", radius: true},
		{name: "radius and certificate", file: "auth = \"radius[config=/etc/radcli/radiusclient.conf,groupconfig=true]\"\nenable-auth = \"certificate\"", radius: true, certificate: true},
		{name: "radius and plain", file: "auth = radius[config=/etc/radcli/radiusclient.conf]\nenable-auth = plain", invalid: true},
		{name: "duplicate alternatives", file: "auth = radius[config=/etc/radcli/radiusclient.conf]\nenable-auth = certificate\nenable-auth = certificate", invalid: true},
		{name: "plain", file: "auth = \"plain[passwd=/etc/ocserv/ocpasswd]\"", invalid: true},
		{name: "fallback", file: "auth = certificate\nenable-auth = plain", invalid: true},
		{name: "mixed", file: "auth = certificate\nauth = radius[config=/etc/radcli/radiusclient.conf]", invalid: true},
		{name: "missing", file: "# auth = certificate", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CUSTOMER_MULTI_NODE_ENABLED", "false")
			require.NoError(t, os.WriteFile(path, []byte(tc.file), 0600))
			var cfg config.Config
			err := cfg.LoadOcservAuth(path)
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.radius, cfg.RadiusAuth)
			require.Equal(t, tc.certificate, cfg.CertificateAuth)
			require.Equal(t, tc.radius, cfg.CustomerMultiNodeEnabled)
		})
	}
}

func TestRadiusConfigurationKeepsSecretAndHTTPSRequirements(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ocserv.conf")
	require.NoError(t, os.WriteFile(path, []byte("auth = radius[config=/etc/radcli/radiusclient.conf]"), 0600))
	t.Setenv("CUSTOMER_MULTI_NODE_ENABLED", "")
	t.Setenv("SECRET_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("AGENT_NODE", "false")
	t.Setenv("AGENT_UPSTREAM_SCHEME", "https")
	t.Setenv("CUSTOMER_RADIUS_REST_SECRET", "fedcba9876543210fedcba9876543210")

	validate := func() error {
		config.Init(false, "", 0)
		require.NoError(t, config.Get().LoadOcservAuth(path))
		require.True(t, config.Get().RadiusAuth)
		require.True(t, config.Get().CustomerMultiNodeEnabled)
		return config.Validate()
	}
	require.NoError(t, validate())

	t.Setenv("CUSTOMER_RADIUS_REST_SECRET", "")
	require.ErrorContains(t, validate(), "CUSTOMER_RADIUS_REST_SECRET")

	t.Setenv("AGENT_NODE", "true")
	t.Setenv("AGENT_API_HTTPS", "true")
	t.Setenv("TLS_CERT_FILE", "/test/cert.pem")
	t.Setenv("TLS_KEY_FILE", "/test/key.pem")
	require.NoError(t, validate())

	t.Setenv("AGENT_API_HTTPS", "false")
	require.ErrorContains(t, validate(), "requires HTTPS")
}
