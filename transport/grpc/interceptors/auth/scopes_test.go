// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import "testing"

func TestScopeRegistry(t *testing.T) {
	r := NewScopeRegistry()

	r.Register("/svc/Get", "read")
	if got, ok := r.Scope("/svc/Get"); !ok || got != "read" {
		t.Fatalf("Scope = %q, ok = %v", got, ok)
	}
	if _, ok := r.Scope("/svc/Unknown"); ok {
		t.Fatal("Scope should return false for unregistered method")
	}
}

func TestScopeRegistry_DenyByDefault(t *testing.T) {
	r := NewScopeRegistry()
	r.Register("/svc/Get", "read")

	// Unregistered method must return ok=false (deny by default)
	_, ok := r.Scope("/svc/Unregistered")
	if ok {
		t.Fatal("unregistered method must return ok=false")
	}
}

func TestScopeRegistry_ScopeNone(t *testing.T) {
	r := NewScopeRegistry()

	// Explicitly register with ScopeNone — this is an intentionally public endpoint
	r.Register("/health.HealthService/Check", ScopeNone)

	scope, ok := r.Scope("/health.HealthService/Check")
	if !ok {
		t.Fatal("explicitly registered ScopeNone method must return ok=true")
	}
	if scope != ScopeNone {
		t.Fatalf("Scope = %q, want ScopeNone", scope)
	}
}

func TestScopeRegistry_RegisterMethods(t *testing.T) {
	r := NewScopeRegistry()
	r.RegisterMethods("write", "/svc/Create", "/svc/Update")

	if got, ok := r.Scope("/svc/Create"); !ok || got != "write" {
		t.Fatalf("Scope = %q, ok = %v", got, ok)
	}
	if got, ok := r.Scope("/svc/Update"); !ok || got != "write" {
		t.Fatalf("Scope = %q, ok = %v", got, ok)
	}
}

func TestScopeRegistry_AllScopes(t *testing.T) {
	r := NewScopeRegistry()
	r.Register("/svc/A", "x")
	r.Register("/svc/B", "y")

	all := r.AllScopes()
	if len(all) != 2 {
		t.Fatalf("len = %d", len(all))
	}

	// Verify it's a copy
	all["/svc/C"] = "z"
	if _, ok := r.Scope("/svc/C"); ok {
		t.Fatal("AllScopes should return a copy")
	}
}

func TestScopeRegistry_Overwrite(t *testing.T) {
	r := NewScopeRegistry()
	r.Register("/svc/Get", "read")
	r.Register("/svc/Get", "admin")
	if got, ok := r.Scope("/svc/Get"); !ok || got != "admin" {
		t.Fatalf("Scope = %q, ok = %v", got, ok)
	}
}

func BenchmarkScopeRegistry_Scope(b *testing.B) {
	r := NewScopeRegistry()
	r.Register("/svc/Get", "read")
	for b.Loop() {
		r.Scope("/svc/Get")
	}
}
