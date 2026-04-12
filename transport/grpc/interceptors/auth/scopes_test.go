// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScopeRegistry(t *testing.T) {
	r := NewScopeRegistry()

	r.Register("/svc/Get", "read")
	got, ok := r.Scope("/svc/Get")
	require.True(t, ok, "Scope = %q, ok = %v", got, ok)
	require.Equal(t, "read", got)
	_, ok = r.Scope("/svc/Unknown")
	require.False(t, ok, "Scope should return false for unregistered method")
}

func TestScopeRegistry_DenyByDefault(t *testing.T) {
	r := NewScopeRegistry()
	r.Register("/svc/Get", "read")

	// Unregistered method must return ok=false (deny by default)
	_, ok := r.Scope("/svc/Unregistered")
	require.False(t, ok, "unregistered method must return ok=false")
}

func TestScopeRegistry_ScopeNone(t *testing.T) {
	r := NewScopeRegistry()

	// Explicitly register with ScopeNone — this is an intentionally public endpoint
	r.Register("/health.HealthService/Check", ScopeNone)

	scope, ok := r.Scope("/health.HealthService/Check")
	require.True(t, ok, "explicitly registered ScopeNone method must return ok=true")
	require.Equal(t, ScopeNone, scope)
}

func TestScopeRegistry_RegisterMethods(t *testing.T) {
	r := NewScopeRegistry()
	r.RegisterMethods("write", "/svc/Create", "/svc/Update")

	got, ok := r.Scope("/svc/Create")
	require.True(t, ok, "Scope = %q, ok = %v", got, ok)
	require.Equal(t, "write", got)
	got, ok = r.Scope("/svc/Update")
	require.True(t, ok, "Scope = %q, ok = %v", got, ok)
	require.Equal(t, "write", got)
}

func TestScopeRegistry_AllScopes(t *testing.T) {
	r := NewScopeRegistry()
	r.Register("/svc/A", "x")
	r.Register("/svc/B", "y")

	require.Equal(t, 2, r.Len())

	got := maps.Collect(r.AllScopes())
	require.Len(t, got, 2)
	require.Equal(t, "x", got["/svc/A"])
	require.Equal(t, "y", got["/svc/B"])
}

func TestScopeRegistry_Overwrite(t *testing.T) {
	r := NewScopeRegistry()
	r.Register("/svc/Get", "read")
	r.Register("/svc/Get", "admin")
	got, ok := r.Scope("/svc/Get")
	require.True(t, ok, "Scope = %q, ok = %v", got, ok)
	require.Equal(t, "admin", got)
}

func TestScopeRegistry_Freeze(t *testing.T) {
	r := NewScopeRegistry()
	r.Register("/svc/Get", "read")
	r.Freeze()

	got, ok := r.Scope("/svc/Get")
	require.True(t, ok)
	require.Equal(t, "read", got)

	_, ok = r.Scope("/svc/Missing")
	require.False(t, ok)

	require.Equal(t, 1, r.Len())
}

func TestScopeRegistry_Freeze_Idempotent(t *testing.T) {
	r := NewScopeRegistry()
	r.Register("/svc/Get", "read")
	r.Freeze()
	r.Freeze() // must not panic

	got, ok := r.Scope("/svc/Get")
	require.True(t, ok)
	require.Equal(t, "read", got)
}

func TestScopeRegistry_RegisterAfterFreeze_Panics(t *testing.T) {
	r := NewScopeRegistry()
	r.Freeze()

	require.Panics(t, func() { r.Register("/svc/X", "x") })
	require.Panics(t, func() { r.RegisterMethods("y", "/svc/Y") })
}

func BenchmarkScopeRegistry_Scope(b *testing.B) {
	r := NewScopeRegistry()
	r.Register("/svc/Get", "read")
	for b.Loop() {
		r.Scope("/svc/Get")
	}
}

func BenchmarkScopeRegistry_Scope_Frozen(b *testing.B) {
	r := NewScopeRegistry()
	r.Register("/svc/Get", "read")
	r.Freeze()
	for b.Loop() {
		r.Scope("/svc/Get")
	}
}
