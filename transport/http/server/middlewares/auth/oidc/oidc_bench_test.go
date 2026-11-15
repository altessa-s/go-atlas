// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/oidc"

	authoidc "github.com/altessa-s/go-atlas/auth/oidc"
)

// BenchmarkAuthFunc_Success measures the per-request adapter overhead on the
// happy path: validator invocation and claims return.
func BenchmarkAuthFunc_Success(b *testing.B) {
	fn := oidc.AuthFunc(fakeValidator{claims: &authoidc.Claims{Subject: "user1"}})
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := fn.Authenticate(ctx, "tok"); err != nil {
			b.Fatal(err)
		}
	}
}
