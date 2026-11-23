package units

import (
	"context"
	"testing"

	federation "github.com/mmtaee/ocserv-dashboard/backend/internal/usecase/customer_federation"
	"github.com/stretchr/testify/require"
)

func TestCustomerGrantSelection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input federation.CustomerInput
		valid bool
	}{
		{name: "single node", input: federation.CustomerInput{Nodes: []string{"master"}}, valid: true},
		{name: "single group", input: federation.CustomerInput{Groups: []string{"group-id"}}, valid: true},
		{name: "missing"},
		{name: "empty node", input: federation.CustomerInput{Nodes: []string{""}}},
		{name: "empty group", input: federation.CustomerInput{Groups: []string{""}}},
		{name: "multiple nodes", input: federation.CustomerInput{Nodes: []string{"master", "agent-1"}}},
		{name: "multiple groups", input: federation.CustomerInput{Groups: []string{"first", "second"}}},
		{name: "mixed", input: federation.CustomerInput{Nodes: []string{"master"}, Groups: []string{"group-id"}}},
		{name: "duplicate", input: federation.CustomerInput{Nodes: []string{"master", "master"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.valid {
				require.NoError(t, tc.input.ValidateGrant())
				return
			}
			// Invalid grant shapes are rejected before any node or database access.
			master := new(federation.Master)
			for _, id := range []string{"", "00000000-0000-0000-0000-000000000001"} {
				require.ErrorIs(t, master.SaveCustomer(context.Background(), id, tc.input), federation.ErrNoGrants)
			}
		})
	}
}
