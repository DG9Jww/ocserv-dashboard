package units

import (
	"context"
	"testing"

	federation "github.com/mmtaee/ocserv-dashboard/backend/internal/usecase/customer_federation"
	"github.com/stretchr/testify/require"
)

func TestCustomerGrantSelection(t *testing.T) {
	policy := "00000000-0000-0000-0000-000000000001"
	for _, tc := range []struct {
		name  string
		input federation.CustomerInput
		valid bool
	}{
		{name: "single node", input: federation.CustomerInput{Node: "master"}, valid: true},
		{name: "single policy", input: federation.CustomerInput{PolicyGroupID: &policy}, valid: true},
		{name: "missing"},
		{name: "mixed", input: federation.CustomerInput{Node: "master", PolicyGroupID: &policy}},
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
