// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scope_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/auth/scope"
)

var benchGranted = []string{"files:read", "files:write", "buckets:read", "tenants:write"}

func BenchmarkExact(b *testing.B) {
	m := scope.Exact()
	for b.Loop() {
		_ = m(benchGranted, "tenants:write")
	}
}

func BenchmarkWildcard(b *testing.B) {
	m := scope.Wildcard(":")
	granted := []string{"buckets:read", "files:*"}
	for b.Loop() {
		_ = m(granted, "files:write:bulk")
	}
}

func BenchmarkRegistryRequired(b *testing.B) {
	r := scope.NewRegistry()
	r.Register("/files.v1.Files/Write", "files:write")
	r.Freeze()
	for b.Loop() {
		_, _ = r.Required("/files.v1.Files/Write")
	}
}

func BenchmarkEnforce(b *testing.B) {
	r := scope.NewRegistry()
	r.Register("/files.v1.Files/Write", "files:write")
	r.Freeze()
	base := scope.ScopeAuthorizer(func(p *principal) []string { return p.scopes }, scope.Exact())
	e := scope.NewEnforcer(r, func(p *principal, required scope.Scope) bool {
		return p.superuser || base(p, required)
	})
	p := &principal{scopes: benchGranted}
	for b.Loop() {
		_ = e.Enforce(p, "/files.v1.Files/Write")
	}
}

func BenchmarkAnyOf(b *testing.B) {
	authorize := scope.AnyOf(
		scope.ScopeAuthorizer(func(p *principal) []string { return p.scopes }, scope.Exact()),
		func(p *principal, _ scope.Scope) bool { return p.superuser },
	)
	p := &principal{scopes: benchGranted}
	for b.Loop() {
		_ = authorize(p, "tenants:write")
	}
}

func BenchmarkAllOf(b *testing.B) {
	authorize := scope.AllOf(
		scope.ScopeAuthorizer(func(p *principal) []string { return p.scopes }, scope.Exact()),
		func(p *principal, _ scope.Scope) bool { return p.superuser },
	)
	p := &principal{scopes: benchGranted, superuser: true}
	for b.Loop() {
		_ = authorize(p, "tenants:write")
	}
}
