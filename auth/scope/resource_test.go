// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scope_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/scope"
)

type resPrincipal struct {
	ID        string
	Scopes    []scope.Scope
	Superuser bool
}

type resDoc struct {
	OwnerID string
}

const resKey = "/docs.v1.Docs/Update"

func newResEnforcer(t *testing.T, authorize scope.ResourceAuthorizer[*resPrincipal, *resDoc]) *scope.ResourceEnforcer[*resPrincipal, *resDoc] {
	t.Helper()
	reg := scope.NewRegistry()
	reg.Register(resKey, "docs:write")
	reg.Register("/docs.v1.Docs/Public", "") // public action
	reg.Freeze()
	return scope.NewResourceEnforcer(reg, authorize)
}

// scopedOwns grants when the principal carries the required scope AND owns the
// resource — the canonical object-level rule.
func scopedOwns() scope.ResourceAuthorizer[*resPrincipal, *resDoc] {
	scoped := scope.LiftAuthorizer[*resPrincipal, *resDoc](
		scope.ScopeAuthorizer(func(p *resPrincipal) []scope.Scope { return p.Scopes }, scope.Exact()),
	)
	owns := func(p *resPrincipal, d *resDoc, _ scope.Scope) bool { return d.OwnerID == p.ID }
	return scope.ResourceAllOf(scoped, owns)
}

func TestResourceEnforce(t *testing.T) {
	t.Parallel()

	owner := &resPrincipal{ID: "u1", Scopes: []scope.Scope{"docs:write"}}
	doc := &resDoc{OwnerID: "u1"}

	cases := []struct {
		name      string
		authorize scope.ResourceAuthorizer[*resPrincipal, *resDoc]
		principal *resPrincipal
		resource  *resDoc
		key       string
		wantErr   error
	}{
		{
			name:      "scope and ownership granted",
			authorize: scopedOwns(),
			principal: owner,
			resource:  doc,
			key:       resKey,
			wantErr:   nil,
		},
		{
			name:      "has scope but not owner",
			authorize: scopedOwns(),
			principal: &resPrincipal{ID: "u2", Scopes: []scope.Scope{"docs:write"}},
			resource:  doc,
			key:       resKey,
			wantErr:   scope.ErrAccessDenied,
		},
		{
			name:      "owner but missing scope",
			authorize: scopedOwns(),
			principal: &resPrincipal{ID: "u1"},
			resource:  doc,
			key:       resKey,
			wantErr:   scope.ErrAccessDenied,
		},
		{
			name:      "unregistered key denied even when authorizer would allow",
			authorize: scope.ResourceAllOf[*resPrincipal, *resDoc](), // identity: always grants
			principal: owner,
			resource:  doc,
			key:       "/docs.v1.Docs/Unknown",
			wantErr:   scope.ErrAccessDenied,
		},
		{
			name:      "public action allowed without consulting resource or principal",
			authorize: scope.ResourceAnyOf[*resPrincipal, *resDoc](), // identity: always denies
			principal: &resPrincipal{},
			resource:  &resDoc{OwnerID: "someone-else"},
			key:       "/docs.v1.Docs/Public",
			wantErr:   nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := newResEnforcer(t, tc.authorize).Enforce(tc.principal, tc.resource, tc.key)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestResourceAnyOfSuperuserBypassesOwnership(t *testing.T) {
	t.Parallel()

	// Superuser is the caller's concept, lifted from the action level; it bypasses
	// the per-resource ownership gate but still cannot reach an unregistered key.
	superuser := scope.LiftAuthorizer[*resPrincipal, *resDoc](
		func(p *resPrincipal, _ scope.Scope) bool { return p.Superuser },
	)
	authorize := scope.ResourceAnyOf(scopedOwns(), superuser)
	enf := newResEnforcer(t, authorize)

	admin := &resPrincipal{ID: "root", Superuser: true}
	foreign := &resDoc{OwnerID: "someone-else"}

	require.NoError(t, enf.Enforce(admin, foreign, resKey))
	require.ErrorIs(t, enf.Enforce(admin, foreign, "/docs.v1.Docs/Unknown"), scope.ErrAccessDenied)
}

func TestResourceCombinatorIdentities(t *testing.T) {
	t.Parallel()

	p := &resPrincipal{}
	d := &resDoc{}

	// AllOf with no authorizers is the AND identity (grants); AnyOf is the OR
	// identity (denies) — mirroring the action-level combinators.
	require.True(t, scope.ResourceAllOf[*resPrincipal, *resDoc]()(p, d, "x"))
	require.False(t, scope.ResourceAnyOf[*resPrincipal, *resDoc]()(p, d, "x"))
}
