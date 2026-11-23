package federation

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
)

var ErrInvalidPolicyGroup = errors.New("customer_policy_invalid")
var ErrPolicyGroupInUse = errors.New("customer_policy_in_use")

func (m *Master) SavePolicyGroup(ctx context.Context, group models.PortalPolicyGroup) error {
	group.Name = strings.TrimSpace(group.Name)
	if group.Name == "" || len(group.Name) > 128 ||
		(group.ID != "" && !ValidID(group.ID)) || len(group.Routes) > 32 {
		return ErrInvalidPolicyGroup
	}
	for _, value := range []*string{&group.PrimaryDNS, &group.SecondaryDNS} {
		*value = strings.TrimSpace(*value)
		if *value == "" {
			continue
		}
		ip, err := netip.ParseAddr(*value)
		if err != nil || !ip.Is4() || ip.IsUnspecified() || ip.IsMulticast() {
			return ErrInvalidPolicyGroup
		}
		*value = ip.String()
	}
	if group.PrimaryDNS == "" && group.SecondaryDNS != "" {
		return ErrInvalidPolicyGroup
	}
	routes := make([]string, 0, len(group.Routes))
	for _, route := range group.Routes {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(route))
		if err != nil || !prefix.Addr().Is4() {
			return ErrInvalidPolicyGroup
		}
		normalized := prefix.Masked().String()
		if !slices.Contains(routes, normalized) {
			routes = append(routes, normalized)
		}
	}
	group.Routes = routes
	if group.ID == "" {
		return m.store.SaveNew(ctx, &group)
	}
	return m.store.Transaction(ctx, func(tx *repository.PortalRepository) error {
		if _, err := tx.PolicyGroup(ctx, group.ID); err != nil {
			return err
		}
		return tx.Save(ctx, &group)
	})
}

func (m *Master) DeletePolicyGroup(ctx context.Context, id string) error {
	if !ValidID(id) {
		return ErrInvalidPolicyGroup
	}
	return m.store.Transaction(ctx, func(tx *repository.PortalRepository) error {
		inUse, err := tx.PolicyGroupInUse(ctx, id)
		if err != nil {
			return err
		}
		if inUse {
			return ErrPolicyGroupInUse
		}
		return tx.DeletePolicyGroup(ctx, id)
	})
}
