package federation

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrInvalid         = errors.New("invalid_customer_configuration")
	ErrUnavailable     = errors.New("customer_nodes_unavailable")
	ErrDefault         = errors.New("customer_default_unavailable")
	ErrNoGrants        = errors.New("customer_grant_required")
	ErrMissingDefaults = errors.New("customer_node_defaults_missing")
	ErrNodeAuth        = errors.New("customer_node_auth_failed")
	ErrNodeTLS         = errors.New("customer_node_tls_failed")
	ErrUsername        = errors.New("customer_invalid_username")
	ErrPassword        = errors.New("customer_invalid_password")
	ErrConflict        = errors.New("customer_account_conflict")
	usernamePattern    = regexp.MustCompile(`^[A-Za-z0-9._-]{2,32}$`)
	groupPattern       = regexp.MustCompile(`^[A-Za-z0-9_-]{1,16}$`)
	idPattern          = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

type Master struct {
	store              *repository.PortalRepository
	agents             *repository.OcservAgentRepository
	local              *Node
	secret             []byte
	client             *http.Client
	radiusTerminations chan radiusTermination
}

func NewMaster(store *repository.PortalRepository, agents *repository.OcservAgentRepository, local *Node, secret string) *Master {
	return &Master{
		store: store, agents: agents, local: local, secret: []byte(secret),
		client: newNodeClient(), radiusTerminations: make(chan radiusTermination, 128),
	}
}

type CustomerInput struct {
	Username      string   `json:"username"`
	Password      string   `json:"password"`
	Nodes         []string `json:"nodes"`
	Groups        []string `json:"groups"`
	DefaultNode   string   `json:"default_node"`
	PolicyGroupID *string  `json:"policy_group_id"`
}

func (input CustomerInput) ValidateGrant() error {
	if len(input.Nodes)+len(input.Groups) != 1 || slices.Contains(input.Nodes, "") || slices.Contains(input.Groups, "") {
		return ErrNoGrants
	}
	return nil
}

type InventoryNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Inventory struct {
	Nodes        []InventoryNode            `json:"nodes"`
	Groups       []models.PortalNodeGroup   `json:"groups"`
	PolicyGroups []models.PortalPolicyGroup `json:"policy_groups"`
	Defaults     []models.PortalNodeDefault `json:"defaults"`
	Customers    []models.PortalCustomer    `json:"customers"`
	Assignments  []models.PortalAssignment  `json:"assignments"`
}

func agentRef(id uint) string { return "agent-" + strconv.FormatUint(uint64(id), 10) }

func (m *Master) nodes(ctx context.Context) ([]InventoryNode, error) {
	agents, err := m.agents.List(ctx)
	if err != nil {
		return nil, err
	}
	nodes := []InventoryNode{{ID: "master", Name: ""}}
	for _, agent := range agents {
		nodes = append(nodes, InventoryNode{ID: agentRef(agent.ID), Name: agent.Name})
	}
	return nodes, nil
}

func (m *Master) Inventory(ctx context.Context) (Inventory, error) {
	var out Inventory
	var err error
	if out.Nodes, err = m.nodes(ctx); err != nil {
		return out, err
	}
	if out.Groups, err = m.store.Groups(ctx); err != nil {
		return out, err
	}
	if out.PolicyGroups, err = m.store.PolicyGroups(ctx); err != nil {
		return out, err
	}
	if out.Defaults, err = m.store.Defaults(ctx); err != nil {
		return out, err
	}
	if out.Customers, err = m.store.Customers(ctx); err != nil {
		return out, err
	}
	out.Assignments, err = m.store.Assignments(ctx, "")
	return out, err
}

func ValidUsername(name string) bool { return usernamePattern.MatchString(name) && name != ".." }
func ValidID(id string) bool         { return idPattern.MatchString(id) }

func validPassword(password string) bool {
	return len(password) >= 2 && len([]rune(password)) <= 32 && len(password) <= 72 &&
		!strings.ContainsAny(password, "\x00\r\n") && password != "Secret-Ocpasswd"
}

func effective(customer models.PortalCustomer, groups []models.PortalNodeGroup) map[string]bool {
	nodes := make(map[string]bool)
	if customer.Locked || customer.DeletedAt != nil {
		return nodes
	}
	// Older accounts can have multiple grants. Keep them usable until an admin saves a single selection.
	for _, node := range customer.Nodes {
		nodes[node] = true
	}
	for _, group := range groups {
		if slices.Contains(customer.Groups, group.ID) {
			for _, node := range group.Nodes {
				nodes[node] = true
			}
		}
	}
	return nodes
}

func (m *Master) synchronize(ctx context.Context, tx *repository.PortalRepository, customer models.PortalCustomer, groups []models.PortalNodeGroup) error {
	desired := effective(customer, groups)
	current, err := tx.Assignments(ctx, customer.ID)
	if err != nil {
		return err
	}
	for _, row := range current {
		want := desired[row.Node]
		remove := customer.DeletedAt != nil
		delete(desired, row.Node)
		if row.Desired == want && row.Remove == remove {
			continue
		}
		row.Desired, row.Remove, row.Version, row.Error, row.RetryAt = want, remove, row.Version+1, "", time.Now().UTC()
		if want {
			row.Applied = row.Version
		}
		if err := tx.Save(ctx, &row); err != nil {
			return err
		}
	}
	for node := range desired {
		row := models.PortalAssignment{CustomerID: customer.ID, Node: node, Desired: true, Version: 1, Applied: 1, RetryAt: time.Now().UTC()}
		if err := tx.Save(ctx, &row); err != nil {
			return err
		}
	}
	return nil
}

func (m *Master) validateNodes(ctx context.Context, refs []string) error {
	nodes, err := m.nodes(ctx)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if !slices.ContainsFunc(nodes, func(n InventoryNode) bool { return n.ID == ref }) {
			return ErrInvalid
		}
	}
	return nil
}

func (m *Master) SaveCustomer(ctx context.Context, id string, input CustomerInput) error {
	if err := input.ValidateGrant(); err != nil {
		return err
	}
	if err := m.validateNodes(ctx, input.Nodes); err != nil {
		return err
	}
	return m.store.Transaction(ctx, func(tx *repository.PortalRepository) error {
		groups, err := tx.Groups(ctx)
		if err != nil {
			return err
		}
		for _, id := range input.Groups {
			if !slices.ContainsFunc(groups, func(g models.PortalNodeGroup) bool { return g.ID == id }) {
				return ErrInvalid
			}
		}
		customer := models.PortalCustomer{Username: input.Username}
		if id != "" {
			if !ValidID(id) {
				return ErrInvalid
			}
			customer, err = tx.Customer(ctx, id)
			if err != nil {
				return err
			}
		} else if !ValidUsername(customer.Username) {
			return ErrUsername
		}
		if customer.DeletedAt != nil {
			return ErrConflict
		}
		if input.PolicyGroupID != nil {
			customer.PolicyGroupID = nil
			if policyID := *input.PolicyGroupID; policyID != "" {
				if !ValidID(policyID) {
					return ErrInvalid
				}
				if _, err := tx.PolicyGroup(ctx, policyID); err != nil {
					return err
				}
				customer.PolicyGroupID = &policyID
			}
		}
		previous := customer
		previous.Locked = false
		previousNodes := effective(previous, groups)
		customer.Nodes, customer.Groups, customer.DefaultNode = append([]string{}, input.Nodes...), append([]string{}, input.Groups...), input.DefaultNode
		requested := customer
		requested.Locked = false
		targets := effective(requested, groups)
		if input.DefaultNode != "" && !targets[input.DefaultNode] {
			return ErrInvalid
		}
		if err := ensureNodeDefaults(ctx, tx, targets, previousNodes); err != nil {
			return err
		}
		if id == "" || input.Password != "" {
			if err := setPassword(&customer, input.Password); err != nil {
				return err
			}
		}
		if id == "" {
			err = tx.SaveNew(ctx, &customer)
		} else {
			err = tx.Save(ctx, &customer)
		}
		if err != nil {
			return err
		}
		return m.synchronize(ctx, tx, customer, groups)
	})
}

func setPassword(customer *models.PortalCustomer, password string) error {
	if !validPassword(password) {
		return ErrPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	customer.PasswordHash = hash
	return nil
}

func (m *Master) SaveGroup(ctx context.Context, group models.PortalNodeGroup) error {
	group.Name = strings.TrimSpace(group.Name)
	group.Nodes = append([]string{}, group.Nodes...)
	if group.Name == "" || len(group.Name) > 128 || (group.ID != "" && !ValidID(group.ID)) {
		return ErrInvalid
	}
	if err := m.validateNodes(ctx, group.Nodes); err != nil {
		return err
	}
	return m.store.Transaction(ctx, func(tx *repository.PortalRepository) error {
		var err error
		if group.ID == "" {
			err = tx.SaveNew(ctx, &group)
		} else {
			err = tx.Save(ctx, &group)
		}
		if err != nil {
			return err
		}
		groups, err := tx.Groups(ctx)
		if err != nil {
			return err
		}
		customers, err := tx.Customers(ctx)
		if err != nil {
			return err
		}
		for _, customer := range customers {
			if slices.Contains(customer.Groups, group.ID) {
				if err := m.synchronize(ctx, tx, customer, groups); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (m *Master) SaveDefaults(ctx context.Context, value models.PortalNodeDefault) error {
	if err := m.validateNodes(ctx, []string{value.Node}); err != nil {
		return err
	}
	if !groupPattern.MatchString(value.Group) || !value.TrafficType.IsValid() || value.TrafficSize < 0 || value.ExpiryDays < 0 || value.ExpiryDays > 36500 {
		return ErrInvalid
	}
	if value.TrafficType == models.Free {
		value.TrafficSize = 0
	}
	return m.store.Save(ctx, &value)
}

func defaultsFor(defaults []models.PortalNodeDefault, node string) (models.PortalNodeDefault, error) {
	index := slices.IndexFunc(defaults, func(d models.PortalNodeDefault) bool { return d.Node == node })
	if index < 0 {
		return models.PortalNodeDefault{}, ErrMissingDefaults
	}
	return defaults[index], nil
}

func ensureNodeDefaults(ctx context.Context, tx *repository.PortalRepository, targets, old map[string]bool) error {
	defaults, err := tx.Defaults(ctx)
	if err != nil {
		return err
	}
	for node := range targets {
		if old[node] {
			continue
		}
		if _, err := defaultsFor(defaults, node); err != nil {
			return err
		}
	}
	return nil
}

func (m *Master) SetCustomerStatus(ctx context.Context, id string, locked, remove bool) error {
	if !ValidID(id) {
		return ErrInvalid
	}
	return m.store.Transaction(ctx, func(tx *repository.PortalRepository) error {
		customer, err := tx.Customer(ctx, id)
		if remove && errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if customer.DeletedAt != nil {
			if remove {
				return nil
			}
			return ErrConflict
		}
		groups, err := tx.Groups(ctx)
		if err != nil {
			return err
		}
		previous := effective(customer, groups)
		customer.Locked = locked || remove
		if remove {
			now := time.Now().UTC()
			customer.DeletedAt = &now
			customer.PasswordHash = []byte{}
			customer.PolicyGroupID = nil
			customer.Nodes, customer.Groups, customer.DefaultNode = []string{}, []string{}, ""
		}
		if err := ensureNodeDefaults(ctx, tx, effective(customer, groups), previous); err != nil {
			return err
		}
		if err := tx.Save(ctx, &customer); err != nil {
			return err
		}
		return m.synchronize(ctx, tx, customer, groups)
	})
}
