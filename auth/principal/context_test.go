// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package principal_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/principal"
)

func TestContext_RoundTrip(t *testing.T) {
	t.Parallel()

	want := principal.Principal{Subject: "alice", Tenant: "acme", Scopes: []string{"files:read"}}
	ctx := principal.NewContext(t.Context(), want)

	got, ok := principal.FromContext(ctx)
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestContext_Absent(t *testing.T) {
	t.Parallel()

	got, ok := principal.FromContext(t.Context())
	require.False(t, ok)
	require.True(t, got.IsZero())
}

func TestContext_InnerOverridesOuter(t *testing.T) {
	t.Parallel()

	ctx := principal.NewContext(t.Context(), principal.Principal{Subject: "outer"})
	ctx = principal.NewContext(ctx, principal.Principal{Subject: "inner"})

	got, ok := principal.FromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "inner", got.Subject)
}

func TestNewContextFrom(t *testing.T) {
	t.Parallel()

	alice := principal.Principal{Subject: "alice", Scopes: []string{"files:read"}}
	tests := []struct {
		name   string
		v      any
		wantOK bool
	}{
		{name: "value", v: alice, wantOK: true},
		{name: "pointer", v: &alice, wantOK: true},
		{name: "nil pointer", v: (*principal.Principal)(nil)},
		{name: "nil", v: nil},
		{name: "application type", v: "user-data"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, stored := principal.NewContextFrom(t.Context(), tc.v)
			require.Equal(t, tc.wantOK, stored)
			got, ok := principal.FromContext(ctx)
			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				require.Equal(t, alice, got)
			} else {
				require.Equal(t, t.Context(), ctx, "ctx must be returned unchanged")
			}
		})
	}
}
