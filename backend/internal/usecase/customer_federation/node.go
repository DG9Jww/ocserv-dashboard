package federation

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/ocserv/configfile"
	account "github.com/mmtaee/ocserv-dashboard/backend/internal/ocserv/user"
	runtime "github.com/mmtaee/ocserv-dashboard/backend/internal/platform/ocserv"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
	"gorm.io/gorm"
)

const (
	gateDirectory    = "/etc/ocserv/managed-customers"
	trustDirectory   = "/etc/ocserv/ssl/portal-trust"
	caFile           = trustDirectory + "/ca.pem"
	crlFile          = trustDirectory + "/crl.pem"
	trustVersionFile = trustDirectory + "/revision"
)

type Node struct {
	store          *repository.PortalRepository
	users          *repository.OcservUserRepository
	systems        *repository.SystemRepository
	admins         *repository.UserRepository
	accounts       *account.OcservUser
	runtime        *runtime.Client
	operator       string
	mu             sync.RWMutex
	loadedCA       []byte
	loadedCRL      []byte
	loadedRevision uint64
}

func NewNode(store *repository.PortalRepository, users *repository.OcservUserRepository, systems *repository.SystemRepository, admins *repository.UserRepository, accounts *account.OcservUser, runtime *runtime.Client, operator string) *Node {
	return &Node{store: store, users: users, systems: systems, admins: admins, accounts: accounts, runtime: runtime, operator: operator}
}

type Trust struct {
	CA  []byte `json:"ca"`
	CRL []byte `json:"crl"`
}

type Apply struct {
	CustomerID   string                   `json:"customer_id"`
	Username     string                   `json:"username"`
	Password     string                   `json:"password,omitempty"`
	Version      uint64                   `json:"version"`
	TrustVersion uint64                   `json:"trust_version"`
	Desired      bool                     `json:"desired"`
	Remove       bool                     `json:"remove,omitempty"`
	Default      models.PortalNodeDefault `json:"default"`
	Trust        Trust                    `json:"trust"`
}

func atomicFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".portal-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(data)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}

func (n *Node) applyTrust(trust Trust, revision uint64) error {
	if revision == 0 {
		return ErrInvalid
	}
	previous, err := os.ReadFile(trustVersionFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		version, err := strconv.ParseUint(strings.TrimSpace(string(previous)), 10, 64)
		if err != nil {
			return err
		}
		if revision < version {
			return ErrPending
		}
	}
	if n.loadedRevision == revision && bytes.Equal(n.loadedCA, trust.CA) && bytes.Equal(n.loadedCRL, trust.CRL) {
		return nil
	}
	caBlock, _ := pem.Decode(trust.CA)
	crlBlock, _ := pem.Decode(trust.CRL)
	if caBlock == nil || crlBlock == nil {
		return ErrInvalid
	}
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil || !ca.IsCA {
		return ErrInvalid
	}
	crl, err := x509.ParseRevocationList(crlBlock.Bytes)
	if err != nil || crl.CheckSignatureFrom(ca) != nil {
		return ErrInvalid
	}
	parent := filepath.Dir(trustDirectory)
	generation, err := os.MkdirTemp(parent, "portal-trust-v-")
	if err != nil {
		return err
	}
	removeGeneration := true
	defer func() {
		if removeGeneration {
			_ = os.RemoveAll(generation)
		}
	}()
	for file, data := range map[string][]byte{
		"ca.pem": trust.CA, "crl.pem": trust.CRL, "revision": []byte(strconv.FormatUint(revision, 10) + "\n"),
	} {
		if err := os.WriteFile(filepath.Join(generation, file), data, 0600); err != nil {
			return err
		}
	}
	previousLink, err := os.Readlink(trustDirectory)
	if err != nil {
		return err
	}
	switchLink := func(target string) error {
		link := filepath.Join(generation, "switch")
		if err := os.Symlink(target, link); err != nil {
			return err
		}
		return os.Rename(link, trustDirectory)
	}
	if err := switchLink(filepath.Base(generation)); err != nil {
		return err
	}
	if _, err := n.runtime.Reload(); err != nil {
		if rollback := switchLink(previousLink); rollback != nil {
			removeGeneration = false
			return errors.Join(err, rollback)
		}
		return err
	}
	removeGeneration = false
	if filepath.Base(previousLink) == previousLink && strings.HasPrefix(previousLink, "portal-trust-v-") {
		_ = os.RemoveAll(filepath.Join(parent, previousLink))
	}
	n.loadedCA, n.loadedCRL, n.loadedRevision = trust.CA, trust.CRL, revision
	return nil
}

func portalConfigReady() bool {
	config, err := os.ReadFile("/etc/ocserv/ocserv.conf")
	if err != nil {
		return false
	}
	directives := map[string]string{
		"connect-script": "/usr/local/lib/ocserv-dashboard/portal-connect-check",
		"ca-cert":        caFile,
		"crl":            crlFile,
		"auth":           `"certificate"`,
		"enable-auth":    `"plain[passwd=/etc/ocserv/ocpasswd]"`,
		"cert-user-oid":  "2.5.4.3",
	}
	hook, err := os.Stat(directives["connect-script"])
	if err != nil || !hook.Mode().IsRegular() || hook.Mode().Perm()&0111 == 0 {
		return false
	}
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(config), "\n") {
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if strings.HasPrefix(key, "[vhost:") {
			return false
		}
		if expected, found := directives[key]; ok && found {
			if seen[key] || strings.TrimSpace(value) != expected {
				return false
			}
			seen[key] = true
		}
	}
	return len(directives) == len(seen)
}

func (n *Node) Apply(ctx context.Context, input Apply) (uint, error) {
	if !ValidID(input.CustomerID) || !ValidUsername(input.Username) || input.Version == 0 || (input.Desired && input.Remove) {
		return 0, ErrInvalid
	}
	if input.Desired && !portalConfigReady() {
		return 0, ErrNodeSetup
	}
	if input.Desired && (input.TrustVersion == 0 || !validPassword(input.Password) || !groupPattern.MatchString(input.Default.Group) ||
		!input.Default.TrafficType.IsValid() || input.Default.TrafficSize < 0 || input.Default.ExpiryDays < 0 || input.Default.ExpiryDays > 36500) {
		return 0, ErrInvalid
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	local, err := n.store.Local(ctx, input.CustomerID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if !input.Desired {
			// Preserve the newer version to fence any delayed provisioning request.
			local = models.PortalLocalAccount{CustomerID: input.CustomerID, Version: input.Version}
			return 0, n.store.Save(ctx, &local)
		}
		return n.createAndApply(ctx, input, local)
	}
	if err != nil {
		return 0, err
	}
	if input.Version < local.Version {
		return local.OcservID, nil
	}
	if local.OcservID == 0 {
		if !input.Desired {
			local.Version = input.Version
			return 0, n.store.Save(ctx, &local)
		}
		return n.createAndApply(ctx, input, local)
	}
	return n.applyExisting(ctx, input, local)
}

func (n *Node) createAndApply(ctx context.Context, input Apply, previous models.PortalLocalAccount) (uint, error) {
	_, err := n.users.GetByUsername(ctx, input.Username)
	if err == nil {
		return 0, ErrConflict
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	_, exists, err := ocpasswdRecord(input.Username)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, ErrConflict
	}
	owner, err := n.admins.GetByUsername(ctx, n.operator)
	if err != nil {
		return 0, err
	}
	if input.Default.Group != "defaults" {
		if info, err := os.Stat(configfile.GroupConfigFilePathCreator(input.Default.Group)); err != nil || !info.Mode().IsRegular() {
			return 0, ErrNodeGroup
		}
	}
	row := &models.OcservUser{OwnerID: owner.ID, Username: input.Username, Password: models.PortalManagedPassword, Group: input.Default.Group,
		TrafficType: input.Default.TrafficType, TrafficSize: input.Default.TrafficSize, ExpiryMode: models.ExpiryModeUnlimited}
	if input.Default.ExpiryDays > 0 {
		t := time.Now().AddDate(0, 0, input.Default.ExpiryDays)
		row.ExpiryMode, row.ExpireAt = models.ExpiryModeFixed, &t
	}
	local, err := n.store.Reserve(ctx, input.CustomerID, row, previous.OcservID == 0 && previous.CustomerID != "")
	if err != nil {
		return 0, err
	}
	return n.applyExisting(ctx, input, local)
}

func (n *Node) applyExisting(ctx context.Context, input Apply, local models.PortalLocalAccount) (uint, error) {
	row, err := n.store.NodeAccount(ctx, local.OcservID)
	if err != nil {
		return 0, err
	}
	if row.Username != input.Username || row.Password != models.PortalManagedPassword {
		return 0, ErrConflict
	}
	if row.DeletedAt.Valid {
		if !input.Desired {
			return row.ID, nil
		}
		return 0, ErrConflict
	}
	if err := os.MkdirAll(gateDirectory, 0700); err != nil {
		return 0, err
	}
	marker := filepath.Join(gateDirectory, row.Username)
	local.Version, local.Desired = input.Version, input.Desired
	if err := n.store.Save(ctx, &local); err != nil {
		return 0, err
	}
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	if !input.Desired {
		if err := n.store.ApplyNodeLock(ctx, local, true); err != nil {
			return row.ID, err
		}
		if err := setOcpasswdState(row.Username, "-l"); err != nil {
			return row.ID, err
		}
		var outcome NodeResult
		if err := n.live(ctx, &outcome, row, "terminate"); err != nil {
			return row.ID, err
		}
		if input.Remove {
			if err := setOcpasswdState(row.Username, "-d"); err != nil {
				return row.ID, err
			}
			if err := n.accounts.DeleteConfig(row.Username); err != nil {
				return row.ID, err
			}
			return row.ID, n.store.ArchiveNodeAccount(ctx, row.ID)
		}
		return row.ID, nil
	}
	if err := n.applyTrust(input.Trust, input.TrustVersion); err != nil {
		return row.ID, err
	}
	if !local.Configured {
		if err := n.accounts.SyncConfig(row.Username, row.Group, row.Config); err != nil {
			return row.ID, err
		}
		local.Configured = true
		if err := n.store.Save(ctx, &local); err != nil {
			return row.ID, err
		}
	}
	// Password edits must not rewrite the user’s runtime configuration or policy.
	if err := n.accounts.SetPassword(row.Group, row.Username, input.Password); err != nil {
		return row.ID, err
	}
	if err := n.store.ApplyNodeLock(ctx, local, false); err != nil {
		return row.ID, err
	}
	row, err = n.users.GetByID(ctx, row.ID)
	if err != nil {
		return 0, err
	}
	if row.IsLocked || row.DeactivatedAt != nil {
		_, err := configfile.RunOcpasswd("-l", "-c", configfile.OcpasswdPath, row.Username)
		if err != nil {
			return row.ID, err
		}
	}
	expiry := int64(0)
	if row.ExpireAt != nil {
		expiry = row.ExpireAt.Unix()
	}
	return row.ID, atomicFile(marker, []byte(strconv.FormatInt(expiry, 10)+"\n"))
}

func ocpasswdRecord(username string) (string, bool, error) {
	existing, err := os.ReadFile(configfile.OcpasswdPath)
	if err != nil {
		return "", false, err
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if name, data, ok := strings.Cut(line, ":"); ok && name == username {
			_, password, _ := strings.Cut(data, ":")
			return password, true, nil
		}
	}
	return "", false, nil
}

func setOcpasswdState(username, flag string) error {
	password, exists, err := ocpasswdRecord(username)
	if err != nil || !exists {
		return err
	}
	if flag == "-l" && strings.HasPrefix(password, "!") {
		return nil
	}
	_, err = configfile.RunOcpasswd(flag, "-c", configfile.OcpasswdPath, username)
	return err
}

type Query struct {
	CustomerID string     `json:"customer_id"`
	OcservID   uint       `json:"ocserv_id"`
	Operation  string     `json:"operation"`
	From       *time.Time `json:"from,omitempty"`
	Until      *time.Time `json:"until,omitempty"`
	Offset     int        `json:"offset,omitempty"`
	Limit      int        `json:"limit,omitempty"`
}

type NodeResult struct {
	LifecycleApplied bool                          `json:"lifecycle_applied,omitempty"`
	OcservID         uint                          `json:"ocserv_id,omitempty"`
	Account          *models.OcservUser            `json:"account,omitempty"`
	Sessions         []models.OnlineUserSession    `json:"sessions,omitempty"`
	Bandwidth        repository.TotalBandwidths    `json:"bandwidth"`
	Daily            []models.DailyTraffic         `json:"daily,omitempty"`
	Activities       []models.OcservUserSessionLog `json:"activities,omitempty"`
	Total            int64                         `json:"total,omitempty"`
	Profile          *NodeProfile                  `json:"profile,omitempty"`
}

type NodeProfile struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
}

func (n *Node) Execute(ctx context.Context, q Query) (NodeResult, error) {
	var out NodeResult
	if !ValidID(q.CustomerID) || q.OcservID == 0 {
		return out, ErrInvalid
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	local, err := n.store.Local(ctx, q.CustomerID)
	if err != nil || local.OcservID != q.OcservID {
		return out, ErrConflict
	}
	switch q.Operation {
	case "bandwidth":
		out.Bandwidth, err = n.users.TotalBandwidthUserDateRange(ctx, q.OcservID, q.From, q.Until)
	case "daily":
		out.Daily, err = n.store.NodeDaily(ctx, q.OcservID, q.From, q.Until)
	case "activity":
		if q.Offset < 0 || q.Limit < 0 || q.Limit > 100 {
			return out, ErrInvalid
		}
		out.Activities, out.Total, err = n.store.NodeActivity(ctx, q.OcservID, q.From, q.Until, q.Offset, q.Limit)
		for i := range out.Activities {
			out.Activities[i].PortalKey = strconv.FormatUint(uint64(out.Activities[i].ID), 10)
		}
	case "account", "profile", "sessions", "disconnect", "terminate":
		if !local.Desired {
			return out, ErrConflict
		}
		row, loadErr := n.users.GetByID(ctx, q.OcservID)
		if loadErr != nil {
			return out, loadErr
		}
		err = n.live(ctx, &out, row, q.Operation)
	default:
		return out, ErrInvalid
	}
	return out, err
}

func (n *Node) live(ctx context.Context, out *NodeResult, row *models.OcservUser, operation string) error {
	var err error
	switch operation {
	case "account":
		// Return only allowlisted account fields to the Master.
		out.Account = &models.OcservUser{Username: row.Username, IsLocked: row.IsLocked, ExpiryMode: row.ExpiryMode, ExpireAt: row.ExpireAt,
			ExpireDaysAfterFirstConnection: row.ExpireDaysAfterFirstConnection, FirstConnectedAt: row.FirstConnectedAt, DeactivatedAt: row.DeactivatedAt,
			TrafficType: row.TrafficType, TrafficSize: row.TrafficSize, RunningRx: row.RunningRx, RunningTx: row.RunningTx}
		out.Account.Owner.Username = row.Owner.Username
	case "profile":
		if row.IsLocked || row.DeactivatedAt != nil || (row.ExpireAt != nil && time.Now().After(*row.ExpireAt)) {
			return ErrDefault
		}
		sys, err := n.systems.System(ctx)
		if err != nil || sys == nil {
			return ErrDefault
		}
		name, err1 := account.NormalizeProfileConnectionName(sys.ClientProfileConnectionName)
		address, err2 := account.NormalizeProfileServerAddress(sys.ClientProfileServerAddress)
		port, err3 := account.NormalizeProfileServerPort(sys.ClientProfileServerPort)
		if err := errors.Join(err1, err2, err3); err != nil {
			return ErrDefault
		}
		out.Profile = &NodeProfile{Name: name, Address: address, Port: port}
	case "sessions", "disconnect", "terminate":
		sessions, err := n.runtime.OnlineSessions()
		if err != nil {
			return err
		}
		for _, session := range sessions {
			if session.Username == row.Username {
				out.Sessions = append(out.Sessions, session)
			}
		}
		if len(out.Sessions) > 0 && operation != "sessions" {
			if operation == "disconnect" {
				_, err = n.runtime.Disconnect(row.Username)
			} else {
				_, err = n.runtime.Terminate(row.Username)
			}
			return err
		}
	default:
		return fmt.Errorf("unknown node operation: %w", ErrInvalid)
	}
	return err
}
