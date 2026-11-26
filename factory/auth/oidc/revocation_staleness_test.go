// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oidc"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
)

// revocation.maxStaleness must reach oidc.WithRevocationMaxStaleness: a bound
// of one nanosecond is already exceeded by the first validation, zero opts
// out, and without a scheduled sync the bound does not apply.
func TestProviderBuilder_RevocationMaxStalenessIsMapped(t *testing.T) {
	t.Parallel()

	require.Equal(t, oidc.DefaultRevocationMaxStaleness, authconfig.DefaultOIDCRevocation().MaxStaleness)

	tests := []struct {
		name         string
		maxStaleness time.Duration
		syncEnabled  bool
		wantErr      error
	}{
		{name: "bound is enforced", maxStaleness: time.Nanosecond, syncEnabled: true, wantErr: oidc.ErrRevocationStale},
		{name: "default bound trusts a fresh sync", maxStaleness: authconfig.DefaultOIDCRevocation().MaxStaleness, syncEnabled: true},
		{name: "zero opts out", syncEnabled: true},
		{name: "no scheduled sync", maxStaleness: time.Nanosecond},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			list := filepath.Join(t.TempDir(), "revoked.txt")
			require.NoError(t, os.WriteFile(list, []byte("another-token\n"), 0o600))

			idp := newCoverageIdP(t)
			rev := memoryFilterRevocation()
			rev.Source = &authconfig.OIDCRevocationSource{File: list}
			rev.SyncEnabled = tc.syncEnabled
			rev.SyncSchedule = "@every 1h"
			rev.MaxStaleness = tc.maxStaleness
			cfg := &authconfig.OIDC{
				DiscoveryUrl: idp.discoveryURL(),
				Validation:   &authconfig.OIDCValidation{Claims: &authconfig.OIDCClaims{Audience: []string{"svc"}}},
				Revocation:   rev,
			}
			p := idp.provider(t, New(cfg))

			_, err := p.ValidateToken(t.Context(), idp.sign(t, nil))
			if tc.wantErr != nil {
				require.ErrorIs(t, err, oidc.ErrRevocationCheck)
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

// jwks.maxStaleness must reach oidc.WithJWKSMaxStaleness only when set: an
// omitted value adds no option (the provider keeps its 24h default), a tiny
// bound is enforced once the IdP is down, and an explicit zero opts out.
func TestProviderBuilder_JWKSMaxStalenessIsMapped(t *testing.T) {
	t.Parallel()

	require.Equal(t, oidc.DefaultJWKSMaxStaleness, *authconfig.DefaultOIDCJwks().MaxStaleness)

	build := func(maxStaleness *time.Duration, idp *coverageIdP) *ProviderBuilder {
		return New(&authconfig.OIDC{
			DiscoveryUrl: idp.discoveryURL(),
			Validation:   &authconfig.OIDCValidation{Claims: &authconfig.OIDCClaims{Audience: []string{"svc"}}},
			JWKS:         &authconfig.OIDCJwks{HTTPTimeout: 5 * time.Second, MaxStaleness: maxStaleness},
		})
	}

	t.Run("omitted adds no option", func(t *testing.T) {
		t.Parallel()
		idp := newCoverageIdP(t)
		omitted, err := build(nil, idp).buildProviderOptions(t.Context())
		require.NoError(t, err)
		set, err := build(new(time.Hour), idp).buildProviderOptions(t.Context())
		require.NoError(t, err)
		require.Len(t, set, len(omitted)+1)
	})

	for _, tc := range []struct {
		name    string
		bound   time.Duration
		wantErr error
	}{
		{name: "bound is enforced when the refresh fails", bound: time.Nanosecond, wantErr: oidc.ErrJWKSStale},
		{name: "explicit zero opts out"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			idp := newCoverageIdP(t)
			p := idp.provider(t, build(&tc.bound, idp))
			idp.down.Store(true)

			_, err := p.ValidateToken(t.Context(), idp.sign(t, nil))
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
