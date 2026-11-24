// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authconfig

import (
	"testing"

	"github.com/stretchr/testify/require"

	egressrule "github.com/altessa-s/go-atlas-net/egressrule"
	httpconfig "github.com/altessa-s/go-atlas/config/http"
)

func TestAuthSSRF_Validate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		cfg     *httpconfig.ClientSSRF
		wantErr string
	}{
		{name: "absent"},
		{name: "empty", cfg: &httpconfig.ClientSSRF{}},
		{name: "allowed CIDRs", cfg: &httpconfig.ClientSSRF{AllowedCIDRs: []string{"10.0.0.0/8", "fd00::/8"}}},
		{name: "rules", cfg: &httpconfig.ClientSSRF{
			Allow: []egressrule.Rule{{CIDR: "10.0.1.0/24", Host: "*.internal", Port: 443}},
			Deny:  []egressrule.Rule{{Host: "blocked.internal"}},
		}},
		{name: "invalid CIDR", cfg: &httpconfig.ClientSSRF{AllowedCIDRs: []string{"bogus"}}, wantErr: "AllowedCIDRs"},
		{name: "invalid rule", cfg: &httpconfig.ClientSSRF{Deny: []egressrule.Rule{{Host: "https://example.com"}}}, wantErr: "deny[0].host"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			configs := map[string]interface{ Validate() error }{
				"oidc":   &OIDC{DiscoveryUrl: "https://idp.example.com/.well-known/openid-configuration", SSRF: tc.cfg},
				"gitlab": &OPAGitLab{Endpoint: "https://gitlab.example.com", Token: "tok", ProjectID: 42, SSRF: tc.cfg},
				"s3":     &OPAS3{Bucket: "policies", SSRF: tc.cfg},
			}
			for name, cfg := range configs {
				t.Run(name, func(t *testing.T) {
					err := cfg.Validate()
					if tc.wantErr == "" {
						require.NoError(t, err)
						return
					}
					require.ErrorContains(t, err, "SSRF")
					require.ErrorContains(t, err, tc.wantErr)
				})
			}
		})
	}
}
