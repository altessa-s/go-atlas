// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package principal_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/principal"
)

// fakeClaims is a principal.ClaimsSource double.
type fakeClaims struct {
	sub    string
	scopes []string
	strs   map[string]string
	slices map[string][]string
}

func (f fakeClaims) Subject() string  { return f.sub }
func (f fakeClaims) Scopes() []string { return f.scopes }
func (f fakeClaims) String(n string) (string, bool) {
	v, ok := f.strs[n]
	return v, ok
}
func (f fakeClaims) StringSlice(n string) ([]string, bool) {
	v, ok := f.slices[n]
	return v, ok
}

func TestScopeAccessors(t *testing.T) {
	t.Parallel()
	p := principal.Principal{Scopes: []string{"files:read", "files:list"}}
	require.True(t, p.HasScope("files:read"))
	require.False(t, p.HasScope("files:write"))
	require.True(t, p.HasAnyScope("files:write", "files:list"))
	require.False(t, p.HasAnyScope())
	require.True(t, p.HasAllScopes("files:read", "files:list"))
	require.True(t, p.HasAllScopes()) // vacuously true
	require.False(t, p.HasAllScopes("files:read", "files:write"))
}

func TestRoleAndClaimAccessors(t *testing.T) {
	t.Parallel()
	p := principal.Principal{Roles: []string{"admin"}, Claims: map[string]any{"dept": "eng"}}
	require.True(t, p.HasRole("admin"))
	require.False(t, p.HasRole("user"))
	require.True(t, p.HasAnyRole("user", "admin"))
	require.False(t, p.HasAnyRole())

	v, ok := p.Claim("dept")
	require.True(t, ok)
	require.Equal(t, "eng", v)
	_, ok = p.Claim("absent")
	require.False(t, ok)
}

func TestIsZero(t *testing.T) {
	t.Parallel()
	require.True(t, principal.Principal{}.IsZero())
	require.False(t, principal.Principal{Subject: "alice"}.IsZero())
	require.False(t, principal.Principal{Scopes: []string{"x"}}.IsZero())
}

func TestFromClaimsDefaults(t *testing.T) {
	t.Parallel()
	c := fakeClaims{
		sub:    "alice",
		scopes: []string{"files:read"},
		strs:   map[string]string{"tenant": "acme"},
		slices: map[string][]string{"roles": {"admin"}},
	}
	p := principal.FromClaims(c)
	require.Equal(t, "alice", p.Subject)
	require.Equal(t, "acme", p.Tenant)
	require.Equal(t, []string{"files:read"}, p.Scopes)
	require.Equal(t, []string{"admin"}, p.Roles)
}

func TestFromClaimsCustomClaimNames(t *testing.T) {
	t.Parallel()
	c := fakeClaims{
		sub:    "bob",
		strs:   map[string]string{"org": "globex"},
		slices: map[string][]string{"groups": {"ops", "sre"}},
	}
	p := principal.FromClaims(c, principal.WithTenantClaim("org"), principal.WithRolesClaim("groups"))
	require.Equal(t, "globex", p.Tenant)
	require.Equal(t, []string{"ops", "sre"}, p.Roles)
}
