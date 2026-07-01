// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/auth/principal"
)

func TestPrincipal(t *testing.T) {
	t.Parallel()
	claims := map[string]any{
		"sub":    "alice",
		"scope":  "files:read files:list",
		"tenant": "acme",
		"roles":  []any{"admin"},
	}
	p := oidc.Principal(claims)
	require.Equal(t, "alice", p.Subject)
	require.Equal(t, "acme", p.Tenant)
	require.Equal(t, []string{"files:read", "files:list"}, p.Scopes)
	require.Equal(t, []string{"admin"}, p.Roles)
}

func TestPrincipalCustomClaims(t *testing.T) {
	t.Parallel()
	claims := map[string]any{"sub": "bob", "org": "globex", "groups": []any{"ops"}}
	p := oidc.Principal(claims, principal.WithTenantClaim("org"), principal.WithRolesClaim("groups"))
	require.Equal(t, "globex", p.Tenant)
	require.Equal(t, []string{"ops"}, p.Roles)
}
