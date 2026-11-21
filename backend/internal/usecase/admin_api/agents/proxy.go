package agents

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/authz"
)

var ErrAgentPath = errors.New("unsupported agent API path")

type Proxy struct {
	agents *Usecase
	client *http.Client
	scheme string
}

func NewProxy(agents *Usecase, scheme string, transport http.RoundTripper) (*Proxy, error) {
	if scheme != "http" && scheme != "https" {
		return nil, errors.New("AGENT_UPSTREAM_SCHEME must be http or https")
	}
	if transport == nil {
		direct := http.DefaultTransport.(*http.Transport).Clone()
		direct.Proxy = nil
		transport = direct
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Proxy{agents: agents, client: client, scheme: scheme}, nil
}

func (p *Proxy) Forward(id uint, nodePath string, incoming *http.Request) (*http.Response, error) {
	if !authz.AgentAPIPath(nodePath) {
		return nil, ErrAgentPath
	}
	agent, err := p.agents.Get(incoming.Context(), id)
	if err != nil {
		return nil, err
	}
	if agent.Port < 1 || agent.Port > 65535 || !validAddress(agent.AddressType, agent.Address) || agent.Token == "" {
		return nil, errors.New("invalid stored agent destination")
	}
	target := url.URL{
		Scheme: p.scheme, Host: net.JoinHostPort(agent.Address, strconv.Itoa(agent.Port)),
		Path: "/api" + nodePath, RawQuery: incoming.URL.RawQuery,
	}
	outgoing, err := http.NewRequestWithContext(incoming.Context(), incoming.Method, target.String(), incoming.Body)
	if err != nil {
		return nil, err
	}
	outgoing.ContentLength = incoming.ContentLength
	for _, header := range []string{"Accept", "Content-Type", "Content-Encoding"} {
		for _, value := range incoming.Header.Values(header) {
			outgoing.Header.Add(header, value)
		}
	}
	outgoing.Header.Set("Authorization", authz.AgentAuthorizationPrefix+agent.Token)
	return p.client.Do(outgoing)
}
