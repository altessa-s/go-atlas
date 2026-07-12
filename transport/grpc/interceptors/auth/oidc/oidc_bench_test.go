// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
)

// BenchmarkAuthFunc_Success measures the token validation happy path through
// AuthFunc: credential extraction, validator invocation, and claims return.
// It reuses the mockValidator fixture from oidc_test.go.
func BenchmarkAuthFunc_Success(b *testing.B) {
	v := &mockValidator{claims: &Claims{Subject: "user1"}}
	fn := AuthFunc(v)

	req := auth.Request{
		Base:    auth.Base{AuthMethod: auth.MethodToken},
		Payload: &auth.TokenCredentials{Token: "valid-token"},
	}
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := fn(ctx, req); err != nil {
			b.Fatal(err)
		}
	}
}
