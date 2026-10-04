// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPClientSSRF_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     *HTTPClientSSRF
		wantErr bool
	}{
		{name: "nil receiver", cfg: nil},
		{name: "no cidrs", cfg: &HTTPClientSSRF{}},
		{name: "valid cidrs", cfg: &HTTPClientSSRF{AllowedCIDRs: []string{"10.0.0.0/8", "fd00::/8"}}},
		{name: "invalid cidr", cfg: &HTTPClientSSRF{AllowedCIDRs: []string{"10.0.0.0/8", "bogus"}}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDefaultHTTPClientSSRF_IsStrict(t *testing.T) {
	t.Parallel()

	cfg := DefaultHTTPClientSSRF()
	require.False(t, cfg.Disabled, "default must keep SSRF protection enabled")

}
