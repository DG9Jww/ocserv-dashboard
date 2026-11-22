package agents

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
)

const (
	agentStatusTTL        = 30 * time.Second
	agentProbeTimeout     = 3 * time.Second
	agentProbeConcurrency = 4
)

type HealthStatus string

const (
	HealthChecking    HealthStatus = "checking"
	HealthOnline      HealthStatus = "online"
	HealthOffline     HealthStatus = "offline"
	HealthUnavailable HealthStatus = "unavailable"
)

type HealthSnapshot struct {
	Statuses   map[uint]HealthStatus `json:"statuses"`
	Refreshing bool                  `json:"refreshing"`
}

type agentHealthKey struct {
	address     string
	addressType models.AgentAddressType
	port        int
	tokenHash   [sha256.Size]byte
}

type agentHealthEntry struct {
	key       agentHealthKey
	status    HealthStatus
	checkedAt time.Time
}

type agentHealthCache struct {
	mu         sync.Mutex
	entries    map[uint]agentHealthEntry
	refreshing bool
}

type agentProbe struct {
	agent models.OcservAgent
	key   agentHealthKey
}

func (p *Proxy) Health(ctx context.Context) (HealthSnapshot, error) {
	nodes, err := p.agents.List(ctx)
	if err != nil {
		return HealthSnapshot{}, err
	}
	cache := &p.health
	cache.mu.Lock()
	defer cache.mu.Unlock()

	now := time.Now()
	entries := make(map[uint]agentHealthEntry, len(nodes))
	statuses := make(map[uint]HealthStatus, len(nodes))
	var pending []agentProbe
	for _, node := range nodes {
		key := agentHealthKey{node.Address, node.AddressType, node.Port, sha256.Sum256([]byte(node.Token))}
		entry := cache.entries[node.ID]
		if entry.status == "" || entry.key != key {
			entry = agentHealthEntry{key: key, status: HealthChecking}
		}
		entries[node.ID] = entry
		statuses[node.ID] = entry.status
		if !cache.refreshing && now.Sub(entry.checkedAt) >= agentStatusTTL {
			pending = append(pending, agentProbe{node, key})
		}
	}
	cache.entries = entries
	if len(pending) > 0 {
		cache.refreshing = true
		// The shared batch must finish even if the caller leaves the page.
		go p.refreshHealth(context.WithoutCancel(ctx), pending)
	}
	return HealthSnapshot{Statuses: statuses, Refreshing: cache.refreshing}, nil
}

func (p *Proxy) refreshHealth(ctx context.Context, pending []agentProbe) {
	cache := &p.health
	jobs := make(chan agentProbe)
	var workers sync.WaitGroup
	for range min(agentProbeConcurrency, len(pending)) {
		workers.Go(func() {
			for job := range jobs {
				status := p.probeHealth(ctx, &job.agent)
				cache.mu.Lock()
				if entry, exists := cache.entries[job.agent.ID]; exists && entry.key == job.key {
					cache.entries[job.agent.ID] = agentHealthEntry{job.key, status, time.Now()}
				}
				cache.mu.Unlock()
			}
		})
	}
	for _, job := range pending {
		jobs <- job
	}
	close(jobs)
	workers.Wait()
	cache.mu.Lock()
	cache.refreshing = false
	cache.mu.Unlock()
}

func (p *Proxy) probeHealth(ctx context.Context, agent *models.OcservAgent) HealthStatus {
	ctx, cancel := context.WithTimeout(ctx, agentProbeTimeout)
	defer cancel()
	request, err := p.agentRequest(ctx, agent, http.MethodGet, "/system", "", nil)
	if err != nil {
		return HealthUnavailable
	}
	request.Header.Set("Accept", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		var network *net.OpError
		var timeout net.Error
		if errors.Is(err, context.DeadlineExceeded) ||
			(errors.As(err, &timeout) && timeout.Timeout()) ||
			(errors.As(err, &network) && network.Op == "dial") {
			return HealthOffline
		}
		return HealthUnavailable
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return HealthUnavailable
	}
	// Validate the response shape without forwarding or retaining system settings.
	var payload struct {
		FirstInit *bool `json:"first_init"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&payload); err != nil || payload.FirstInit == nil {
		return HealthUnavailable
	}
	return HealthOnline
}
