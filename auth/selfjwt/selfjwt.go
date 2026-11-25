// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

// New builds a [Minter] and a [Verifier] that share the same [KeyProvider],
// issuer, audience and options. Use it instead of separate [NewMinter] /
// [NewVerifier] calls when a single process both issues and verifies tokens: it
// guarantees the two sides agree on the settings they have in common — the
// issuer and audience (the minter writes iss and aud, the verifier checks them)
// and the clock — so they cannot drift apart.
func New(src KeyProvider, issuer, audience string, opts ...PairOption) (*Minter, *Verifier) {
	o := newPairOptions(opts...)
	m := newMinter(src, issuer, audience, &minterOptions{
		maxTokenLifetime:  o.maxTokenLifetime,
		allowedAlgorithms: o.allowedAlgorithms,
		rand:              o.rand,
		clock:             o.clock,
		metrics:           o.metrics,
	})
	v := newVerifier(src, issuer, audience, &verifierOptions{
		leeway:               o.leeway,
		cacheTTL:             o.cacheTTL,
		cacheMaxEntries:      o.cacheMaxEntries,
		keyLookupTimeout:     o.keyLookupTimeout,
		negativeCacheTTL:     o.negativeCacheTTL,
		maxConcurrentLookups: o.maxConcurrentLookups,
		allowedAlgorithms:    o.allowedAlgorithms,
		clock:                o.clock,
		metrics:              o.metrics,
		revocation:           o.revocation,
	})
	return m, v
}
