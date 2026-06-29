// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scope_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/scope"
)

func yesAuthorizer(*principal, scope.Scope) bool { return true }
func noAuthorizer(*principal, scope.Scope) bool  { return false }

func TestAnyOf(t *testing.T) {
	t.Parallel()
	require.True(t, scope.AnyOf(noAuthorizer, yesAuthorizer)(&principal{}, "x"))
	require.False(t, scope.AnyOf(noAuthorizer, noAuthorizer)(&principal{}, "x"))
	require.False(t, scope.AnyOf[*principal]()(&principal{}, "x"), "empty AnyOf denies")
}

func TestAllOf(t *testing.T) {
	t.Parallel()
	require.True(t, scope.AllOf(yesAuthorizer, yesAuthorizer)(&principal{}, "x"))
	require.False(t, scope.AllOf(yesAuthorizer, noAuthorizer)(&principal{}, "x"))
	require.True(t, scope.AllOf[*principal]()(&principal{}, "x"), "empty AllOf grants")
}

func TestAnyOfSuperuserComposition(t *testing.T) {
	t.Parallel()
	base := scope.ScopeAuthorizer(func(p *principal) []string { return p.scopes }, scope.Exact())
	superuser := func(p *principal, _ scope.Scope) bool { return p.superuser }
	authorize := scope.AnyOf(base, superuser)

	require.True(t, authorize(&principal{scopes: []string{"files:write"}}, "files:write"))
	require.True(t, authorize(&principal{superuser: true}, "files:write"))
	require.False(t, authorize(&principal{}, "files:write"))
}

func TestAllOfExtraGate(t *testing.T) {
	t.Parallel()
	base := scope.ScopeAuthorizer(func(p *principal) []string { return p.scopes }, scope.Exact())
	tenantActive := func(p *principal, _ scope.Scope) bool { return p.superuser } // reuse flag as the gate
	authorize := scope.AllOf(base, tenantActive)

	require.True(t, authorize(&principal{scopes: []string{"files:write"}, superuser: true}, "files:write"))
	require.False(t, authorize(&principal{scopes: []string{"files:write"}}, "files:write"), "gate closed")
	require.False(t, authorize(&principal{superuser: true}, "files:write"), "missing scope")
}
