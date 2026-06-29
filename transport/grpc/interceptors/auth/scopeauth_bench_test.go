// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/auth/scope"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
)

func BenchmarkScopeClientAuth(b *testing.B) {
	reg := scope.NewRegistry()
	reg.Register(methodWrite, "files:write")
	reg.Freeze()
	enf := scope.NewEnforcer(reg, scope.ScopeAuthorizer(
		func(p *testPrincipal) []string { return p.scopes },
		scope.Exact(),
	))
	ca := auth.ScopeClientAuth(enf)
	ctx := context.Background()
	cred := credsFor(methodWrite, &testPrincipal{scopes: []string{"files:write"}})

	for b.Loop() {
		_, _ = ca.ClientAuth(ctx, cred)
	}
}
