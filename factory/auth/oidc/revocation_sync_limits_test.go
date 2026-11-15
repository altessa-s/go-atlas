// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
)

// revocationListCfg returns an OIDC config whose jti revocation list is served
// by handler on the test IdP.
func revocationListCfg(t *testing.T, idp *coverageIdP, handler http.HandlerFunc) *authconfig.OIDC {
	t.Helper()
	mux, ok := idp.srv.Config.Handler.(*http.ServeMux)
	require.True(t, ok)
	mux.HandleFunc("/revoked", handler)

	rev := memoryFilterRevocation()
	rev.ItemType = oidc.RevocationItemTypeJTI
	rev.Source = &authconfig.OIDCRevocationSource{URL: idp.srv.URL + "/revoked"}
	return &authconfig.OIDC{
		DiscoveryUrl: idp.discoveryURL(),
		Validation:   &authconfig.OIDCValidation{Claims: &authconfig.OIDCClaims{Audience: []string{"svc"}}},
		Revocation:   rev,
	}
}

func TestOIDCRevocationSyncDefaults_MirrorProviderDefaults(t *testing.T) {
	t.Parallel()

	def := authconfig.DefaultOIDCRevocation()
	require.Equal(t, oidc.DefaultRevocationSyncTimeout, def.SyncTimeout)
	require.Equal(t, oidc.DefaultRevocationMaxResponseBytes, def.MaxResponseBytes)
	require.False(t, def.AllowEmpty)
}

// revocation.maxResponseBytes must reach the URL loader: a list over the cap
// fails the initial sync, and zero falls back to the loader's default.
func TestProviderBuilder_RevocationMaxResponseBytesIsMapped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		maxBytes int64
		wantErr  error
	}{
		{name: "list over the cap fails the sync", maxBytes: 16, wantErr: oidc.ErrRevocationLoadFailed},
		{name: "list within the cap", maxBytes: 1 << 10},
		{name: "zero selects the loader default"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			idp := newCoverageIdP(t)
			cfg := revocationListCfg(t, idp, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(strings.Repeat("j1\n", 32)))
			})
			cfg.Revocation.MaxResponseBytes = tc.maxBytes

			opts, err := New(cfg).buildProviderOptions(t.Context())
			require.NoError(t, err)
			opts = append(opts,
				oidc.WithDiscoveryValidationMode(oidc.DiscoveryValidationModeDisabled),
				oidc.WithHTTPClient(idp.srv.Client()))
			p, err := oidc.NewProvider(t.Context(), idp.discoveryURL(), opts...)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			t.Cleanup(p.Close)
		})
	}
}

// revocation.syncTimeout must reach oidc.WithRevocationSyncTimeout: a
// scheduled sync against a source that hangs is cut off after the timeout.
func TestProviderBuilder_RevocationSyncTimeoutIsMapped(t *testing.T) {
	t.Parallel()

	var hang atomic.Bool
	var cutOff atomic.Int32
	idp := newCoverageIdP(t)
	cfg := revocationListCfg(t, idp, func(w http.ResponseWriter, r *http.Request) {
		if hang.Load() {
			<-r.Context().Done()
			cutOff.Add(1)
			return
		}
		_, _ = w.Write([]byte("j1\n"))
	})
	cfg.Revocation.SyncEnabled = true
	cfg.Revocation.SyncSchedule = "@every 1s"
	cfg.Revocation.SyncTimeout = 50 * time.Millisecond
	idp.provider(t, New(cfg))

	hang.Store(true)
	testhelpers.WaitFor(t, 10*time.Second, func() bool { return cutOff.Load() > 0 },
		"a hanging scheduled sync must be canceled by revocation.syncTimeout")
}

// revocation.allowEmpty must reach oidc.WithRevocationAllowEmpty: only with it
// does a list that turns up empty replace the current revocation set.
func TestProviderBuilder_RevocationAllowEmptyIsMapped(t *testing.T) {
	t.Parallel()

	for _, allowEmpty := range []bool{false, true} {
		name := "empty list keeps the current set"
		if allowEmpty {
			name = "empty list is authoritative"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var empty atomic.Bool
			var emptyServed atomic.Int32
			idp := newCoverageIdP(t)
			cfg := revocationListCfg(t, idp, func(w http.ResponseWriter, _ *http.Request) {
				if empty.Load() {
					emptyServed.Add(1)
					return
				}
				_, _ = w.Write([]byte("j1\n"))
			})
			cfg.Revocation.SyncEnabled = true
			cfg.Revocation.SyncSchedule = "@every 1s"
			cfg.Revocation.AllowEmpty = allowEmpty
			p := idp.provider(t, New(cfg))

			token := idp.sign(t, map[string]any{"jti": "j1"})
			_, err := p.ValidateToken(t.Context(), token)
			require.ErrorIs(t, err, oidc.ErrTokenRevoked)

			empty.Store(true)
			if allowEmpty {
				testhelpers.WaitFor(t, 10*time.Second, func() bool {
					_, err := p.ValidateToken(t.Context(), token)
					return err == nil
				}, "an empty list must un-revoke the token when revocation.allowEmpty is set")
				return
			}
			// The second empty response is requested only after the sync that
			// got the first one has finished.
			testhelpers.WaitFor(t, 10*time.Second, func() bool { return emptyServed.Load() >= 2 },
				"scheduled syncs must keep fetching the list")
			_, err = p.ValidateToken(t.Context(), token)
			require.ErrorIs(t, err, oidc.ErrTokenRevoked)
		})
	}
}
