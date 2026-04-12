// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
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

	all := r.AllScopes()
	require.Len(t, all, 2)

	// Verify it's a copy
	all["/svc/C"] = "z"
	_, ok := r.Scope("/svc/C")
	require.False(t, ok, "AllScopes should return a copy")
}

func TestScopeRegistry_Overwrite(t *testing.T) {
	r := NewScopeRegistry()
	r.Register("/svc/Get", "read")
	r.Register("/svc/Get", "admin")
	got, ok := r.Scope("/svc/Get")
	require.True(t, ok, "Scope = %q, ok = %v", got, ok)
	require.Equal(t, "admin", got)
}

func BenchmarkScopeRegistry_Scope(b *testing.B) {
	r := NewScopeRegistry()
	r.Register("/svc/Get", "read")
	for b.Loop() {
		r.Scope("/svc/Get")
	}
}
