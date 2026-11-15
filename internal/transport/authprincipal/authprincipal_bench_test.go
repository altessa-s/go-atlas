// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authprincipal_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/auth/principal"
	"github.com/altessa-s/go-atlas/internal/transport/authprincipal"
)

// BenchmarkBridge measures the per-request cost of exposing an authenticator
// result through principal.FromContext, read back once as a handler would.
func BenchmarkBridge(b *testing.B) {
	alice := principal.Principal{Subject: "alice", Tenant: "acme", Scopes: []string{"files:read"}}
	cases := []struct {
		name string
		data any
	}{
		{name: "value", data: alice},
		{name: "pointer", data: &alice},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			ctx := b.Context()
			b.ReportAllocs()
			for b.Loop() {
				if _, ok := principal.FromContext(authprincipal.Bridge(ctx, tc.data)); !ok {
					b.Fatal("principal not bridged")
				}
			}
		})
	}
}
