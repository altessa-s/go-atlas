// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scope_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/auth/scope"
)

func BenchmarkResourceEnforcerEnforce(b *testing.B) {
	reg := scope.NewRegistry()
	reg.Register(resKey, "docs:write")
	reg.Freeze()

	scoped := scope.LiftAuthorizer[*resPrincipal, *resDoc](
		scope.ScopeAuthorizer(func(p *resPrincipal) []scope.Scope { return p.Scopes }, scope.Exact()),
	)
	owns := func(p *resPrincipal, d *resDoc, _ scope.Scope) bool { return d.OwnerID == p.ID }
	enf := scope.NewResourceEnforcer(reg, scope.ResourceAllOf(scoped, owns))

	p := &resPrincipal{ID: "u1", Scopes: []scope.Scope{"docs:write"}}
	d := &resDoc{OwnerID: "u1"}

	b.ReportAllocs()
	for b.Loop() {
		_ = enf.Enforce(p, d, resKey)
	}
}
