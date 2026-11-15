// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authprincipal_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/principal"
	"github.com/altessa-s/go-atlas/internal/transport/authprincipal"
)

// claimsSource maps itself onto a principal, as *oidc.Claims does.
type claimsSource struct{ p principal.Principal }

func (c claimsSource) Principal() principal.Principal { return c.p }

func TestBridge(t *testing.T) {
	t.Parallel()

	alice := principal.Principal{Subject: "alice"}
	tests := []struct {
		name   string
		data   any
		wantOK bool
	}{
		{name: "value", data: alice, wantOK: true},
		{name: "pointer", data: &alice, wantOK: true},
		{name: "nil pointer", data: (*principal.Principal)(nil)},
		{name: "nil", data: nil},
		{name: "application type", data: "user-data"},
		{name: "claims source", data: claimsSource{p: alice}, wantOK: true},
		{name: "anonymous claims source", data: claimsSource{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := principal.FromContext(authprincipal.Bridge(t.Context(), tc.data))
			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				require.Equal(t, alice, got)
			}
		})
	}
}
