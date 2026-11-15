// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import (
	"testing"

	"github.com/stretchr/testify/require"

	egressrule "github.com/altessa-s/go-atlas-net/egressrule"
)

func TestHTTPClientSSRF_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     *ClientSSRF
		wantErr bool
	}{
		{name: "nil receiver", cfg: nil},
		{name: "no cidrs", cfg: &ClientSSRF{}},
		{name: "valid cidrs", cfg: &ClientSSRF{AllowedCIDRs: []string{"10.0.0.0/8", "fd00::/8"}}},
		{name: "invalid cidr", cfg: &ClientSSRF{AllowedCIDRs: []string{"10.0.0.0/8", "bogus"}}, wantErr: true},
		{name: "valid rules", cfg: &ClientSSRF{
			Allow: []egressrule.Rule{{Name: "internalAPI", CIDR: "10.0.1.0/24", Host: "*.internal", Port: 443}, {CIDR: "::ffff:10.0.2.0/120"}},
			Deny:  []egressrule.Rule{{Host: "*.Internal"}, {CIDR: "169.254.0.0/16"}, {Port: 22}},
		}},
		{name: "allow without cidr", cfg: &ClientSSRF{Allow: []egressrule.Rule{{Host: "a.example"}}}, wantErr: true},
		{name: "deny without matcher", cfg: &ClientSSRF{Deny: []egressrule.Rule{{Name: "empty"}}}, wantErr: true},
		{name: "unparseable rule cidr", cfg: &ClientSSRF{Deny: []egressrule.Rule{{CIDR: "bogus"}}}, wantErr: true},
		{name: "short mapped prefix", cfg: &ClientSSRF{Allow: []egressrule.Rule{{CIDR: "::ffff:0:0/64"}}}, wantErr: true},
		{name: "port out of range", cfg: &ClientSSRF{Deny: []egressrule.Rule{{Port: 70000}}}, wantErr: true},
		{name: "invalid name", cfg: &ClientSSRF{Deny: []egressrule.Rule{{Name: "has space", Port: 22}}}, wantErr: true},
		{name: "host with scheme", cfg: &ClientSSRF{Deny: []egressrule.Rule{{Host: "https://a.example"}}}, wantErr: true},
		{name: "host trailing dot", cfg: &ClientSSRF{Deny: []egressrule.Rule{{Host: "a.example."}}}, wantErr: true},
		{name: "unicode glob label", cfg: &ClientSSRF{Deny: []egressrule.Rule{{Host: "bü*.example"}}}, wantErr: true},
		{name: "soft hyphen label", cfg: &ClientSSRF{Deny: []egressrule.Rule{{Host: "example.\u00ad"}}}, wantErr: true},
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

	cfg := DefaultClientSSRF()
	require.False(t, cfg.Disabled, "default must keep SSRF protection enabled")
}

func TestHTTPClientSSRF_ValidateNamesRule(t *testing.T) {
	t.Parallel()

	cfg := &ClientSSRF{Deny: []egressrule.Rule{{Port: 22}, {Host: "a:80"}}}
	require.ErrorContains(t, cfg.Validate(), "deny[1].host")
}
