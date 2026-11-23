package federation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	account "github.com/mmtaee/ocserv-dashboard/backend/internal/ocserv/user"
	runtime "github.com/mmtaee/ocserv-dashboard/backend/internal/platform/ocserv"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
)

type Node struct {
	systems *repository.SystemRepository
	runtime *runtime.Client
}

func NewNode(systems *repository.SystemRepository, runtime *runtime.Client) *Node {
	return &Node{systems: systems, runtime: runtime}
}

type Query struct {
	Operation      string `json:"operation"`
	RadiusUsername string `json:"radius_username"`
}

type NodeProfile struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
}

type NodeResult struct {
	Sessions []models.OnlineUserSession `json:"sessions,omitempty"`
	Profile  *NodeProfile               `json:"profile,omitempty"`
}

func (n *Node) Execute(ctx context.Context, q Query) (NodeResult, error) {
	var result NodeResult
	if !ValidUsername(q.RadiusUsername) {
		return result, ErrInvalid
	}
	switch q.Operation {
	case "terminate":
		reply, err := n.runtime.Terminate(q.RadiusUsername)
		if strings.TrimSpace(reply) == fmt.Sprintf("could not terminate user '%s'", q.RadiusUsername) {
			err = nil
		}
		return result, err
	case "sessions", "disconnect":
		sessions, err := n.runtime.OnlineSessions()
		if err != nil {
			return result, err
		}
		for _, session := range sessions {
			if session.Username == q.RadiusUsername {
				result.Sessions = append(result.Sessions, session)
			}
		}
		if q.Operation == "disconnect" && len(result.Sessions) != 0 {
			_, err := n.runtime.Disconnect(q.RadiusUsername)
			return result, err
		}
		return result, nil
	case "profile":
		sys, err := n.systems.System(ctx)
		if err != nil || sys == nil {
			return result, ErrDefault
		}
		name, err1 := account.NormalizeProfileConnectionName(sys.ClientProfileConnectionName)
		address, err2 := account.NormalizeProfileServerAddress(sys.ClientProfileServerAddress)
		port, err3 := account.NormalizeProfileServerPort(sys.ClientProfileServerPort)
		if errors.Join(err1, err2, err3) != nil {
			return result, ErrDefault
		}
		result.Profile = &NodeProfile{Name: name, Address: address, Port: port}
		return result, nil
	default:
		return result, ErrInvalid
	}
}
