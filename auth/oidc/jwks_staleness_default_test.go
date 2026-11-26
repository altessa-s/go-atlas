// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The JWKS staleness bound is on by default (fail closed after 24h); an
// explicit zero or the disabled failure mode opts out.
func TestJWKSStaleness_DefaultBound(t *testing.T) {
	t.Parallel()

	require.Equal(t, 24*time.Hour, DefaultJWKSMaxStaleness)

	tests := []struct {
		name    string
		opts    []Option
		age     time.Duration
		down    bool
		wantErr error
	}{
		{name: "past the default bound, IdP down", age: DefaultJWKSMaxStaleness + time.Minute, down: true, wantErr: ErrJWKSStale},
		{name: "past the default bound, IdP healthy refreshes once", age: DefaultJWKSMaxStaleness + time.Minute},
		{name: "within the default bound", age: DefaultJWKSMaxStaleness - time.Minute},
		{name: "zero opts out", opts: []Option{WithJWKSMaxStaleness(0)}, age: 10 * DefaultJWKSMaxStaleness},
		{name: "disabled mode opts out", opts: []Option{WithJWKSFailureMode(JWKSFailureModeDisabled)}, age: 10 * DefaultJWKSMaxStaleness},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			idp := newTestIdP(t)
			raw := idp.sign(t, idp.claims(nil))
			p := idp.newProvider(t, tc.opts...)
			setStaleness(t, p, tc.age)
			if tc.down {
				idp.jwksStatus.Store(http.StatusInternalServerError)
			}

			_, err := p.ValidateToken(t.Context(), raw)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.age > DefaultJWKSMaxStaleness && len(tc.opts) == 0 {
				require.Less(t, p.keys.clock.elapsed(p.keys.lastJWKSRefresh.Load().observedAt), time.Minute,
					"a stale cache is refreshed before the token is rejected")
			}
		})
	}
}
