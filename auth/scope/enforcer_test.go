// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scope_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/scope"
)

// principal is a minimal test identity. The core knows nothing about it; the
// Authorizer supplied by the test extracts scopes and the superuser flag.
type principal struct {
	scopes    []string
	superuser bool
}

const (
	methodWrite  = "/files.v1.Files/Write"
	methodHealth = "/health.v1.Health/Check"
	methodSecret = "/files.v1.Files/Unregistered"
)

func newEnforcer(t *testing.T, m scope.Matcher) *scope.Enforcer[*principal] {
	t.Helper()
	reg := scope.NewRegistry()
	reg.Register(methodWrite, "files:write")
	reg.Register(methodHealth, "") // public
	reg.Freeze()

	base := scope.ScopeAuthorizer(func(p *principal) []string { return p.scopes }, m)
	authorize := func(p *principal, required scope.Scope) bool {
		return p.superuser || base(p, required)
	}
	return scope.NewEnforcer(reg, authorize)
}

func TestEnforceAllowsWithScope(t *testing.T) {
	t.Parallel()
	e := newEnforcer(t, scope.Exact())
	require.NoError(t, e.Enforce(&principal{scopes: []string{"files:write"}}, methodWrite))
}

func TestEnforceDeniesMissingScope(t *testing.T) {
	t.Parallel()
	e := newEnforcer(t, scope.Exact())
	require.ErrorIs(t, e.Enforce(&principal{scopes: []string{"files:read"}}, methodWrite), scope.ErrAccessDenied)
}

func TestEnforceDeniesUnregistered(t *testing.T) {
	t.Parallel()
	e := newEnforcer(t, scope.Exact())
	require.ErrorIs(t, e.Enforce(&principal{scopes: []string{"files:write"}}, methodSecret), scope.ErrAccessDenied)
}

func TestEnforceAllowsPublic(t *testing.T) {
	t.Parallel()
	e := newEnforcer(t, scope.Exact())
	require.NoError(t, e.Enforce(&principal{}, methodHealth))
}

func TestEnforceSuperuserAllowsRegistered(t *testing.T) {
	t.Parallel()
	e := newEnforcer(t, scope.Exact())
	require.NoError(t, e.Enforce(&principal{superuser: true}, methodWrite))
}

func TestEnforceSuperuserStillDeniedUnregistered(t *testing.T) {
	t.Parallel()
	e := newEnforcer(t, scope.Exact())
	// Composed superuser bypass does not reach unregistered keys: deny-by-default
	// is decided before the Authorizer runs (fail-closed).
	require.ErrorIs(t, e.Enforce(&principal{superuser: true}, methodSecret), scope.ErrAccessDenied)
}

func TestEnforceWildcardGrant(t *testing.T) {
	t.Parallel()
	e := newEnforcer(t, scope.Wildcard(":"))
	require.NoError(t, e.Enforce(&principal{scopes: []string{"files:*"}}, methodWrite))
	require.ErrorIs(t, e.Enforce(&principal{scopes: []string{"buckets:*"}}, methodWrite), scope.ErrAccessDenied)
}
