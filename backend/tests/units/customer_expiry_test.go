package units

import (
	"testing"
	"time"

	federation "github.com/mmtaee/ocserv-dashboard/backend/internal/usecase/customer_federation"
	"github.com/stretchr/testify/require"
)

func TestCustomerExpiryDate(t *testing.T) {
	for _, test := range []struct {
		date     string
		zone     string
		expected string
	}{
		{"2026-12-24", "Asia/Shanghai", "2026-12-24T15:59:59.999Z"},
		{"2026-03-08", "America/New_York", "2026-03-09T03:59:59.999Z"},
		{"2028-02-29", "UTC", "2028-02-29T23:59:59.999Z"},
		{"2026-02-29", "UTC", ""},
		{"2026-12-24", "Not/A_Zone", ""},
	} {
		t.Run(test.date+"/"+test.zone, func(t *testing.T) {
			input := federation.CustomerInput{ExpiryDate: &test.date, ExpiryZone: test.zone}
			expiry, err := input.ResolveExpiry()
			if test.expected == "" {
				require.ErrorIs(t, err, federation.ErrInvalid)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.expected, expiry.Format(time.RFC3339Nano))
		})
	}
	legacy := time.Date(2026, time.December, 24, 10, 0, 0, 0, time.UTC)
	input := federation.CustomerInput{ExpiresAt: &legacy}
	expiry, err := input.ResolveExpiry()
	require.NoError(t, err)
	require.Equal(t, &legacy, expiry)
	blank := ""
	input.ExpiryDate = &blank
	expiry, err = input.ResolveExpiry()
	require.NoError(t, err)
	require.Nil(t, expiry)
}
