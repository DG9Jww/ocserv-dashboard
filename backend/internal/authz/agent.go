package authz

import (
	"path"
	"strings"
)

const AgentAuthorizationPrefix = "OcservAgent "

// AgentAPIPath limits node credentials and the Master proxy to node operations.
func AgentAPIPath(value string) bool {
	if value == "" || value != path.Clean(value) || strings.ContainsAny(value, "\\%\x00") {
		return false
	}
	switch value {
	case "/system/users/login", "/system/users/profile", "/system/users/password":
		return false
	case "/system", "/system/ocserv-config", "/system/release":
		return true
	}
	for _, prefix := range []string{"/home", "/ocserv/users", "/ocserv/groups", "/occtl", "/reports", "/backup", "/systemd", "/system/users", "/telegram"} {
		if value == prefix || strings.HasPrefix(value, prefix+"/") {
			return true
		}
	}
	return false
}
