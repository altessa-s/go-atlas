// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import "testing"

// BenchmarkValidateToken compares a full signature verification with a
// signature-cache hit; both run the complete claim policy and revocation step.
func BenchmarkValidateToken(b *testing.B) {
	idp := newTestIdP(b)
	raw := idp.sign(b, idp.claims(nil))

	b.Run("no-cache", func(b *testing.B) {
		p := idp.newProvider(b)
		for b.Loop() {
			if _, err := p.ValidateToken(b.Context(), raw); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("cache-hit", func(b *testing.B) {
		p := idp.newProvider(b, WithTokenCache(newMemCacher()))
		if _, err := p.ValidateToken(b.Context(), raw); err != nil {
			b.Fatal(err)
		}
		for b.Loop() {
			if _, err := p.ValidateToken(b.Context(), raw); err != nil {
				b.Fatal(err)
			}
		}
	})
}
