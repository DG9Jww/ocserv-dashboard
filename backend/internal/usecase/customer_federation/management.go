package federation

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrEmail        = errors.New("customer_invalid_email")
	ErrInvalid      = errors.New("invalid_customer_configuration")
	ErrUnavailable  = errors.New("customer_nodes_unavailable")
	ErrDefault      = errors.New("customer_default_unavailable")
	ErrNoGrants     = errors.New("customer_grant_required")
	ErrNodeAuth     = errors.New("customer_node_auth_failed")
	ErrNodeTLS      = errors.New("customer_node_tls_failed")
	ErrUsername     = errors.New("customer_invalid_username")
	ErrPassword     = errors.New("customer_invalid_password")
	ErrConflict     = errors.New("customer_account_conflict")
	usernamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{2,32}$`)
	idPattern       = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
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
	Username      string             `json:"username"`
	Email         *string            `json:"email"`
	Password      string             `json:"password"`
	Node          string             `json:"node"`
	TrafficType   models.TrafficType `json:"traffic_type"`
	TrafficSize   int64              `json:"traffic_size"`
	ExpiresAt     *time.Time         `json:"expires_at"`
	ExpiryDate    *string            `json:"expiry_date"`
	ExpiryZone    string             `json:"expiry_timezone"`
	DefaultNode   string             `json:"default_node"`
	PolicyGroupID *string            `json:"policy_group_id"`
}

// ResolveExpiry keeps timestamp-based requests compatible and resolves calendar dates
// against the time zone selected by the administrator.
func (input CustomerInput) ResolveExpiry() (*time.Time, error) {
	if input.ExpiryDate == nil {
		return input.ExpiresAt, nil
	}
	if *input.ExpiryDate == "" {
		return nil, nil
	}
	date, err := time.Parse(time.DateOnly, *input.ExpiryDate)
	if err != nil || date.Year() < 1 || input.ExpiryZone == "" || input.ExpiryZone == "Local" {
		return nil, ErrInvalid
	}
	zone, err := time.LoadLocation(input.ExpiryZone)
	if err != nil {
		return nil, ErrInvalid
	}
	end := time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 999_000_000, zone)
	if end.Year() != date.Year() || end.Month() != date.Month() || end.Day() != date.Day() {
		return nil, ErrInvalid
	}
	utc := end.UTC()
	if utc.Year() < 1 || utc.Year() > 9999 {
		return nil, ErrInvalid
	}
	return &utc, nil
}

func (input CustomerInput) ValidateGrant() error {
	if (input.Node != "") == (input.PolicyGroupID != nil) {
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
	PolicyGroups []models.PortalPolicyGroup `json:"policy_groups"`
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

	if out.PolicyGroups, err = m.store.PolicyGroups(ctx); err != nil {
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

func (m *Master) synchronize(ctx context.Context, tx *repository.PortalRepository, customer models.PortalCustomer, nodes []string) error {
	desired := make(map[string]bool, len(nodes))
	if !customer.Locked && customer.DeletedAt == nil {
		for _, node := range nodes {
			desired[node] = true
		}
	}
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
	if len(refs) == 0 {
		return nil
	}
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

func (m *Master) grantNodes(ctx context.Context, tx *repository.PortalRepository, node string, policyID *string) ([]string, error) {
	if policyID == nil {
		return []string{node}, m.validateNodes(ctx, []string{node})
	}
	if !ValidID(*policyID) {
		return nil, ErrInvalid
	}
	policy, err := tx.PolicyGroup(ctx, *policyID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvalid
	}
	return policy.Nodes, err
}

func (m *Master) SaveCustomer(ctx context.Context, id string, input CustomerInput) error {
	if err := input.ValidateGrant(); err != nil {
		return err
	}
	updateLimits := id == "" || input.TrafficType != ""
	if input.TrafficType == "" {
		input.TrafficType = models.Free
	}
	if input.TrafficType == models.Free {
		input.TrafficSize = 0
	}
	if !input.TrafficType.IsValid() || input.TrafficSize < 0 {
		return ErrInvalid
	}
	var email string
	if input.Email != nil {
		email = strings.TrimSpace(*input.Email)
		if email != "" {
			address, err := mail.ParseAddress(email)
			if err != nil || address.Address != email || len(email) > 254 {
				return ErrEmail
			}
		}
	}
	expiresAt, err := input.ResolveExpiry()
	if err != nil {
		return err
	}
	return m.store.Transaction(ctx, func(tx *repository.PortalRepository) error {
		nodes, err := m.grantNodes(ctx, tx, input.Node, input.PolicyGroupID)
		if err != nil {
			return err
		}
		if input.DefaultNode != "" && !slices.Contains(nodes, input.DefaultNode) {
			return ErrInvalid
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
			if customer.DeletedAt != nil {
				return ErrConflict
			}
		} else if !ValidUsername(customer.Username) {
			return ErrUsername
		}
		customer.Node, customer.PolicyGroupID, customer.DefaultNode = input.Node, input.PolicyGroupID, input.DefaultNode
		if input.Email != nil {
			customer.Email = email
		}
		if updateLimits {
			customer.TrafficType, customer.TrafficSize = input.TrafficType, input.TrafficSize
		}
		if updateLimits || input.ExpiryDate != nil {
			customer.ExpiresAt = expiresAt
		}
		if id == "" || input.Password != "" {
			if err := setPassword(&customer, input.Password); err != nil {
				return err
			}
		}
		if id == "" {
			err = tx.SaveNew(ctx, &customer)
		} else {
			err = tx.UpdateCustomer(ctx, &customer)
		}
		if err != nil {
			return err
		}
		return m.synchronize(ctx, tx, customer, nodes)
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
		customer.Locked = locked || remove
		if remove {
			now := time.Now().UTC()
			customer.DeletedAt = &now
			customer.PasswordHash = []byte{}
			customer.Email = ""
			customer.PolicyGroupID = nil
			customer.Node, customer.DefaultNode = "", ""
		}
		var nodes []string
		if !customer.Locked {
			nodes, err = m.grantNodes(ctx, tx, customer.Node, customer.PolicyGroupID)
			if err != nil {
				return err
			}
			if !slices.Contains(nodes, customer.DefaultNode) {
				customer.DefaultNode = ""
			}
		}
		if err := tx.UpdateCustomer(ctx, &customer); err != nil {
			return err
		}
		return m.synchronize(ctx, tx, customer, nodes)
	})
}
