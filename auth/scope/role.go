// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scope

import (
	"slices"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

// RoleScopes is an immutable role→scopes table. It maps each role name to the
// scopes that role grants, so a service that models access as roles can reuse the
// scope [Enforcer] without writing its own role-expansion logic.
//
// The table holds only the role→scope mapping — the service's policy. It stays
// deliberately ignorant of what a principal is and how roles are attached to one:
// the caller supplies a rolesOf extractor (see [RoleScopesOf] / [RoleAuthorizer]),
// so no role or identity field is baked into the core.
//
// Build it once with [NewRoleScopes]; it is then safe for concurrent reads.
type RoleScopes struct {
	table *coremaps.ImmutableMap[string, []Scope]
}

// NewRoleScopes builds an immutable [RoleScopes] from a role→scopes mapping. Each
// scope slice is cloned, so later mutation of the caller's map or slices does not
// affect the table. A nil or empty mapping yields a table that grants nothing.
func NewRoleScopes(mapping map[string][]Scope) *RoleScopes {
	table := make(map[string][]Scope, len(mapping))
	for role, scopes := range mapping {
		table[role] = slices.Clone(scopes)
	}
	return &RoleScopes{table: coremaps.NewImmutableMap(table)}
}

// ScopesFor returns the union of the scopes granted by the given roles, in
// first-seen order and with duplicates removed. Unknown roles contribute nothing.
// A nil receiver or no roles yields nil, so it composes safely with a missing
// table.
func (rs *RoleScopes) ScopesFor(roles ...string) []Scope {
	if rs == nil || len(roles) == 0 {
		return nil
	}
	var out []Scope
	seen := make(map[Scope]struct{})
	for _, role := range roles {
		scopes, ok := rs.table.Get(role)
		if !ok {
			continue
		}
		for _, s := range scopes {
			if _, dup := seen[s]; dup {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// RoleScopesOf adapts a caller's role extractor into the scopesOf function that
// [ScopeAuthorizer] expects: it reads the principal's roles with rolesOf and
// expands them through rs. Roles stay entirely on the caller's side — rolesOf is
// where the principal's role field is read; this package never names one.
func RoleScopesOf[P any](rolesOf func(P) []string, rs *RoleScopes) func(P) []Scope {
	return func(p P) []Scope {
		return rs.ScopesFor(rolesOf(p)...)
	}
}

// RoleAuthorizer builds an [Authorizer] that grants access when the scopes
// expanded from a principal's roles satisfy the required scope under m. It is a
// thin convenience over [ScopeAuthorizer] and [RoleScopesOf]:
//
//	scope.RoleAuthorizer(rolesOf, rs, m) == scope.ScopeAuthorizer(scope.RoleScopesOf(rolesOf, rs), m)
//
// Compose it with [AnyOf] / [AllOf] to add a superuser bypass or a tenant gate,
// exactly as with a plain scope authorizer.
func RoleAuthorizer[P any](rolesOf func(P) []string, rs *RoleScopes, m Matcher) Authorizer[P] {
	return ScopeAuthorizer(RoleScopesOf(rolesOf, rs), m)
}
