// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authconfig_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
)

func TestMTLSValidateTrustDomains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		domains []string
		wantErr bool
	}{
		{name: "unset"},
		{name: "valid", domains: []string{"example.org", "prod_1.corp-internal"}},
		{name: "empty entry", domains: []string{""}, wantErr: true},
		{name: "empty entry among valid ones", domains: []string{"example.org", ""}, wantErr: true},
		{name: "blank entry", domains: []string{"  "}, wantErr: true},
		{name: "padded entry", domains: []string{" example.org"}, wantErr: true},
		{name: "upper case", domains: []string{"Example.org"}, wantErr: true},
		{name: "SPIFFE ID instead of a domain", domains: []string{"spiffe://example.org"}, wantErr: true},
		{name: "port", domains: []string{"example.org:443"}, wantErr: true},
		{name: "too long", domains: []string{strings.Repeat("a", 256)}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := authconfig.DefaultMTLS()
			cfg.TrustDomains = tc.domains
			err := cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
