package federation

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/authz"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	logger "github.com/mmtaee/ocserv-dashboard/backend/internal/platform/logging"
)

func newNodeClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{Transport: transport, Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (m *Master) nodeCall(ctx context.Context, node, endpoint string, input any, out *NodeResult) error {
	id, err := strconv.ParseUint(strings.TrimPrefix(node, "agent-"), 10, 64)
	if err != nil || node != agentRef(uint(id)) {
		return ErrInvalid
	}
	agent, err := m.agents.GetByID(ctx, uint(id))
	if err != nil {
		return err
	}
	target := url.URL{Scheme: "https", Host: net.JoinHostPort(agent.Address, strconv.Itoa(agent.Port)), Path: "/api/customer-node/" + endpoint}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authz.AgentAuthorizationPrefix+agent.Token)
	resp, err := m.client.Do(req)
	if err != nil {
		var certificate *tls.CertificateVerificationError
		if errors.As(err, &certificate) {
			return ErrNodeTLS
		}
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusConflict:
		return ErrConflict
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrNodeAuth
	case http.StatusNotFound:
		if endpoint == "apply" {
			return ErrNodeSetup
		}
		return ErrDefault
	case http.StatusBadRequest:
		var failure struct {
			Message []string `json:"message"`
		}
		if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&failure) == nil && len(failure.Message) == 1 {
			for _, known := range []error{ErrNodeSetup, ErrNodeGroup} {
				if failure.Message[0] == known.Error() {
					return known
				}
			}
		}
		return ErrInvalid
	case http.StatusOK:
		return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
	default:
		return ErrUnavailable
	}
}

func (m *Master) query(ctx context.Context, a models.PortalAssignment, q Query) (NodeResult, error) {
	q.CustomerID, q.OcservID = a.CustomerID, a.OcservID
	if a.Node == "master" {
		return m.local.Execute(ctx, q)
	}
	var result NodeResult
	err := m.nodeCall(ctx, a.Node, "query", q, &result)
	return result, err
}

func (m *Master) Run(ctx context.Context) error {
	poll := time.NewTicker(3 * time.Second)
	defer poll.Stop()
	for {
		m.processJobs(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-poll.C:
		}
	}
}

func (m *Master) processJobs(ctx context.Context) {
	jobs, err := m.store.Jobs(ctx, time.Now().UTC())
	if err != nil {
		logger.Warn("customer sync queue unavailable: %v", err)
		return
	}
	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		localID, err := m.applyJob(ctx, job)
		if err != nil {
			code := provisioningError(err)
			logger.Warn("customer sync failed for node %s, account %s: %s", job.Node, job.CustomerID, code)
			if saveErr := m.store.Finish(ctx, job, map[string]any{"error": code, "retry_at": time.Now().UTC().Add(15 * time.Second)}); saveErr != nil {
				logger.Warn("unable to record customer sync failure: %v", saveErr)
			}
			continue
		}
		fields := map[string]any{"applied": job.Version, "error": ""}
		if localID != 0 {
			fields["ocserv_id"] = localID
		}
		if err := m.store.Finish(ctx, job, fields); err != nil {
			logger.Warn("unable to record customer sync completion: %v", err)
		}
	}
}

func (m *Master) applyJob(ctx context.Context, job models.PortalAssignment) (uint, error) {
	customer, err := m.store.Customer(ctx, job.CustomerID)
	if err != nil {
		return 0, err
	}
	if job.Desired && (customer.Locked || customer.DeletedAt != nil) {
		return 0, ErrPending
	}
	input := Apply{CustomerID: customer.ID, Username: customer.Username, Version: job.Version, Desired: job.Desired, Remove: job.Remove}
	if job.Desired {
		defaults, err := m.store.Defaults(ctx)
		if err != nil {
			return 0, err
		}
		input.Default, err = defaultsFor(defaults, job.Node)
		if err != nil {
			return 0, err
		}
		password, err := m.decrypt(customer.PasswordBox, "password:"+customer.Username)
		if err != nil {
			return 0, err
		}
		input.Password = string(password)
		input.TrustVersion = input.Default.TrustVersion
		certScope := scope(input.Default)
		if _, err = m.issue(ctx, customer, certScope); err != nil {
			return 0, err
		}
		input.Trust, err = m.trust(ctx, certScope)
		if err != nil {
			return 0, err
		}
	}
	if job.Node == "master" {
		return m.local.Apply(ctx, input)
	}
	var result NodeResult
	err = m.nodeCall(ctx, job.Node, "apply", input, &result)
	if err == nil && !job.Desired && !result.LifecycleApplied {
		return 0, ErrNodeLifecycle
	}
	return result.OcservID, err
}

func provisioningError(err error) string {
	for _, known := range []error{ErrConflict, ErrInvalid, ErrMissingDefaults, ErrNodeSetup, ErrNodeLifecycle, ErrNodeGroup, ErrNodeAuth, ErrNodeTLS} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return ErrUnavailable.Error()
}
