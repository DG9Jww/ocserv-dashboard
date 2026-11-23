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
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
)

func newNodeClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{Transport: transport, Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (m *Master) nodeCall(ctx context.Context, node string, input Query, out *NodeResult) error {
	id, err := strconv.ParseUint(strings.TrimPrefix(node, "agent-"), 10, 64)
	if err != nil || node != agentRef(uint(id)) {
		return ErrInvalid
	}
	agent, err := m.agents.GetByID(ctx, uint(id))
	if err != nil {
		return err
	}
	target := url.URL{Scheme: "https", Host: net.JoinHostPort(agent.Address, strconv.Itoa(agent.Port)), Path: "/api/customer-node/query"}
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
	case http.StatusOK:
		return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
	default:
		return ErrUnavailable
	}
}

func (m *Master) query(ctx context.Context, node string, q Query) (NodeResult, error) {
	if node == "master" {
		return m.local.Execute(ctx, q)
	}
	var result NodeResult
	err := m.nodeCall(ctx, node, q, &result)
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
		case pending := <-m.radiusTerminations:
			m.terminateRadiusSession(ctx, pending)
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
		err := m.applyJob(ctx, job)
		if err != nil {
			code := provisioningError(err)
			logger.Warn("customer sync failed for node %s, account %s: %s", job.Node, job.CustomerID, code)
			if saveErr := m.store.Finish(ctx, job, map[string]any{"error": code, "retry_at": time.Now().UTC().Add(15 * time.Second)}); saveErr != nil {
				logger.Warn("unable to record customer sync failure: %v", saveErr)
			}
			continue
		}
		fields := map[string]any{"applied": job.Version, "error": ""}
		if err := m.store.Finish(ctx, job, fields); err != nil {
			logger.Warn("unable to record customer sync completion: %v", err)
		}
	}
	m.purgeCompletedCustomers(ctx)
}

func (m *Master) applyJob(ctx context.Context, job models.PortalAssignment) error {
	customer, err := m.store.Customer(ctx, job.CustomerID)
	if err != nil {
		return err
	}
	if job.Desired {
		return nil
	}
	_, err = m.query(ctx, job.Node, Query{Operation: "terminate", RadiusUsername: customer.Username})
	return err
}

func (m *Master) purgeCompletedCustomers(ctx context.Context) {
	customers, err := m.store.CompletedCustomerDeletions(ctx)
	if err != nil {
		logger.Warn("failed to find completed customer deletions: %v", err)
		return
	}
	for _, customer := range customers {
		err := m.store.Transaction(ctx, func(tx *repository.PortalRepository) error {
			return tx.PurgeCustomer(ctx, customer.ID)
		})
		if err != nil {
			logger.Warn("failed to finalize customer deletion: %v", err)
		}
	}
}

func provisioningError(err error) string {
	for _, known := range []error{ErrInvalid, ErrNodeAuth, ErrNodeTLS} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return ErrUnavailable.Error()
}
