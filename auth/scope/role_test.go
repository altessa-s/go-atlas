// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scope_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/scope"
)

type rbacPrincipal struct {
	roles     []string
	superuser bool
}

func rbacRoles(p *rbacPrincipal) []string { return p.roles }

func newRBACScopes() *scope.RoleScopes {
	return scope.NewRoleScopes(map[string][]scope.Scope{
		"viewer": {"files:read"},
		"editor": {"files:read", "files:write"},
		"admin":  {"files:read", "files:write", "users:manage"},
	})
}

func TestRoleScopesScopesFor(t *testing.T) {
	t.Parallel()
	rs := newRBACScopes()

	cases := []struct {
		name  string
		roles []string
		want  []scope.Scope
	}{
		{"single role", []string{"viewer"}, []scope.Scope{"files:read"}},
		{"union dedups", []string{"viewer", "editor"}, []scope.Scope{"files:read", "files:write"}},
		{"unknown role ignored", []string{"viewer", "ghost"}, []scope.Scope{"files:read"}},
		{"all unknown", []string{"ghost"}, nil},
		{"no roles", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, rs.ScopesFor(tc.roles...))
		})
	}
}

func TestRoleScopesNilReceiver(t *testing.T) {
	t.Parallel()
	var rs *scope.RoleScopes
	require.Nil(t, rs.ScopesFor("viewer"))
}

func TestNewRoleScopesClonesInput(t *testing.T) {
	t.Parallel()
	src := map[string][]scope.Scope{"viewer": {"files:read"}}
	rs := scope.NewRoleScopes(src)

	src["viewer"][0] = "mutated"
	src["viewer"] = append(src["viewer"], "files:write")

	require.Equal(t, []scope.Scope{"files:read"}, rs.ScopesFor("viewer"))
}

func TestRoleScopesOfAdapter(t *testing.T) {
	t.Parallel()
	scopesOf := scope.RoleScopesOf(rbacRoles, newRBACScopes())
	require.Equal(t, []scope.Scope{"files:read", "files:write"}, scopesOf(&rbacPrincipal{roles: []string{"editor"}}))
}

func TestRoleAuthorizer(t *testing.T) {
	t.Parallel()
	authz := scope.RoleAuthorizer(rbacRoles, newRBACScopes(), scope.Exact())

	require.True(t, authz(&rbacPrincipal{roles: []string{"editor"}}, "files:write"))
	require.False(t, authz(&rbacPrincipal{roles: []string{"viewer"}}, "files:write"))
	require.False(t, authz(&rbacPrincipal{roles: nil}, "files:read"))
}

// RoleAuthorizer composes with the existing combinators: roles stay caller-side
// and a superuser bypass is OR-ed in without the core knowing about it.
func TestRoleAuthorizerComposesSuperuser(t *testing.T) {
	t.Parallel()
	authz := scope.AnyOf(
		scope.RoleAuthorizer(rbacRoles, newRBACScopes(), scope.Exact()),
		func(p *rbacPrincipal, _ scope.Scope) bool { return p.superuser },
	)
	require.True(t, authz(&rbacPrincipal{superuser: true}, "users:manage"))
	require.False(t, authz(&rbacPrincipal{roles: []string{"viewer"}}, "users:manage"))
}

func TestRoleAuthorizerWithEnforcer(t *testing.T) {
	t.Parallel()
	reg := scope.NewRegistry()
	reg.Register("/files.v1.Files/Write", "files:write")
	reg.Freeze()

	enf := scope.NewEnforcer(reg, scope.RoleAuthorizer(rbacRoles, newRBACScopes(), scope.Exact()))

	require.NoError(t, enf.Enforce(&rbacPrincipal{roles: []string{"editor"}}, "/files.v1.Files/Write"))
	require.ErrorIs(t, enf.Enforce(&rbacPrincipal{roles: []string{"viewer"}}, "/files.v1.Files/Write"), scope.ErrAccessDenied)
}
