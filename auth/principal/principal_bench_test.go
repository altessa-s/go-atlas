// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package principal_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/auth/principal"
)

func BenchmarkHasScope(b *testing.B) {
	p := principal.Principal{Scopes: []string{"a", "b", "files:read", "c", "d"}}
	b.ResetTimer()
	for b.Loop() {
		_ = p.HasScope("files:read")
	}
}

// BenchmarkFromClaims measures the claim→Principal mapping as auth adapters
// run it per request, with default and custom claim-name configurations.
func BenchmarkFromClaims(b *testing.B) {
	c := fakeClaims{
		sub:    "alice",
		scopes: []string{"files:read", "files:list"},
		strs:   map[string]string{"tenant": "acme", "org": "acme"},
		slices: map[string][]string{"roles": {"admin"}, "groups": {"admin"}},
	}

	b.Run("defaults", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = principal.FromClaims(c)
		}
	})

	b.Run("custom_claims", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = principal.FromClaims(c, principal.WithTenantClaim("org"), principal.WithRolesClaim("groups"))
		}
	})

	b.Run("mapper_custom_claims", func(b *testing.B) {
		m := principal.NewMapper(principal.WithTenantClaim("org"), principal.WithRolesClaim("groups"))
		b.ReportAllocs()
		for b.Loop() {
			_ = m.FromClaims(c)
		}
	})
}
