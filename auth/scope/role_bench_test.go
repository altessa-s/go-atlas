// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scope_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/auth/scope"
)

func BenchmarkRoleScopesFor(b *testing.B) {
	rs := scope.NewRoleScopes(map[string][]scope.Scope{
		"viewer": {"files:read"},
		"editor": {"files:read", "files:write"},
		"admin":  {"files:read", "files:write", "users:manage"},
	})
	roles := []string{"viewer", "editor", "admin"}
	b.ReportAllocs()
	for b.Loop() {
		_ = rs.ScopesFor(roles...)
	}
}

func BenchmarkRoleAuthorizer(b *testing.B) {
	rs := scope.NewRoleScopes(map[string][]scope.Scope{
		"editor": {"files:read", "files:write"},
	})
	authz := scope.RoleAuthorizer(func(p *rbacPrincipal) []string { return p.roles }, rs, scope.Exact())
	p := &rbacPrincipal{roles: []string{"editor"}}
	b.ReportAllocs()
	for b.Loop() {
		_ = authz(p, "files:write")
	}
}
