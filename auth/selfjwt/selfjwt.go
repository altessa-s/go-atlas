// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

// New builds a [Minter] and a [Verifier] that share the same [KeyProvider] and
// the same options. Use it instead of separate [NewMinter] / [NewVerifier]
// calls when a single process both issues and verifies tokens: it guarantees
// the two sides agree on the settings they have in common — most importantly the
// issuer (the minter writes the iss claim, the verifier checks it) and the clock
// — so they cannot drift apart through divergent option lists.
//
// Options that only one side consumes are harmless to the other: the minter
// ignores WithAllowedAlgorithms / WithLeeway, and the verifier ignores
// WithMaxTokenLifetime / WithRand.
func New(src KeyProvider, opts ...Option) (*Minter, *Verifier) {
	return NewMinter(src, opts...), NewVerifier(src, opts...)
}
