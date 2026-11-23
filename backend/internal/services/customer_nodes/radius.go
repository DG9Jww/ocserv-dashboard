package customernodes

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	logger "github.com/mmtaee/ocserv-dashboard/backend/internal/platform/logging"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
	federation "github.com/mmtaee/ocserv-dashboard/backend/internal/usecase/customer_federation"
)

type radiusBackend interface {
	AuthorizeRadius(context.Context, string, string, string, string) (federation.RadiusPolicy, error)
	AccountRadius(context.Context, repository.RadiusAccounting) error
}

func RunRadius(ctx context.Context, master *federation.Master, secret string, docker bool) error {
	address := "127.0.0.1:18120"
	if docker {
		address = "0.0.0.0:18120"
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler: RadiusHandler(master, secret), ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout: 5 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second,
	}
	stop := context.AfterFunc(ctx, func() { _ = srv.Close() })
	defer stop()
	if err := srv.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

type radiusAttributes map[string]struct {
	Value []json.RawMessage `json:"value"`
}

// FreeRADIUS 3 REST sends enum attributes as strings and may send integers as numbers.
func (a radiusAttributes) field(name string) string {
	values := a[name].Value
	if len(values) != 1 {
		return ""
	}
	var s string
	if json.Unmarshal(values[0], &s) == nil {
		return s
	}
	return string(values[0])
}

func (a radiusAttributes) counter(direction string) (int64, error) {
	low, err := strconv.ParseUint(a.field("Acct-"+direction+"-Octets"), 10, 32)
	if err != nil {
		return 0, err
	}
	high := uint64(0)
	if raw := a.field("Acct-" + direction + "-Gigawords"); raw != "" {
		high, err = strconv.ParseUint(raw, 10, 31)
		if err != nil {
			return 0, err
		}
	}
	return int64(high<<32 | low), nil
}

func (a radiusAttributes) accounting(now time.Time) (repository.RadiusAccounting, error) {
	p := repository.RadiusAccounting{Node: a.field("NAS-Identifier"), SessionID: a.field("Acct-Session-Id"), Username: a.field("User-Name"), At: now}
	if !federation.ValidUsername(p.Username) || p.Node == "" || len(p.Node) > 32 || p.SessionID == "" || len(p.SessionID) > 128 {
		return p, federation.ErrInvalid
	}
	switch a.field("Acct-Status-Type") {
	case "Start", "1":
		p.Event = models.EventHandshake
	case "Interim-Update", "3":
		p.Event = models.EventPeriodicStats
	case "Stop", "2":
		p.Event = models.EventDisconnect
	default:
		return p, federation.ErrInvalid
	}
	if delay := a.field("Acct-Delay-Time"); delay != "" {
		seconds, err := strconv.ParseUint(delay, 10, 32)
		if err != nil {
			return p, federation.ErrInvalid
		}
		p.At = now.Add(-time.Duration(seconds) * time.Second)
	}
	if ip, err := netip.ParseAddr(a.field("Calling-Station-Id")); err == nil {
		p.IP = ip.String()
	}
	if p.Event != models.EventHandshake {
		var err error
		p.RX, err = a.counter("Input")
		if err != nil {
			return p, federation.ErrInvalid
		}
		p.TX, err = a.counter("Output")
		if err != nil {
			return p, federation.ErrInvalid
		}
	}
	return p, nil
}

func RadiusHandler(master radiusBackend, secret string) http.Handler {
	expected := sha256.Sum256([]byte(secret))
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user, password, ok := req.BasicAuth()
		found := sha256.Sum256([]byte(password))
		if !ok || user != "ocserv-dashboard" || subtle.ConstantTimeCompare(found[:], expected[:]) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if req.URL.Path != "/authorize" && req.URL.Path != "/accounting" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if req.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var attrs radiusAttributes
		decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 32<<10))
		if decoder.Decode(&attrs) != nil || decoder.Decode(new(any)) != io.EOF {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(req.Context(), 8*time.Second)
		defer cancel()
		if req.URL.Path == "/accounting" {
			packet, err := attrs.accounting(time.Now().UTC())
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
			} else if err := master.AccountRadius(ctx, packet); err != nil {
				logger.Warn("RADIUS accounting failed for node %q, event %q: %v", packet.Node, packet.Event, err)
				w.WriteHeader(http.StatusServiceUnavailable)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
			return
		}
		policy, err := master.AuthorizeRadius(ctx, attrs.field("NAS-Identifier"), attrs.field("User-Name"), attrs.field("User-Password"), attrs.field("Acct-Session-Id"))
		switch {
		case errors.Is(err, federation.ErrRadiusRejected):
			w.WriteHeader(http.StatusUnauthorized)
		case err != nil:
			w.WriteHeader(http.StatusServiceUnavailable)
		case policy.Group == "" && policy.SessionTimeout == 0:
			w.WriteHeader(http.StatusNoContent)
		default:
			reply := map[string]any{}
			if policy.Group != "" {
				reply["reply:Class"] = "OU=" + policy.Group
			}
			if policy.SessionTimeout > 0 {
				reply["reply:Session-Timeout"] = policy.SessionTimeout
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(reply)
		}
	})
}
