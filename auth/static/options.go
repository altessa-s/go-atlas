// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

// options holds the configuration for [InMemoryStore].
type options struct {
	// initialTokens seeds the store with token→data pairs before the first lookup.
	initialTokens map[string]any

	// metrics records validation outcomes, latency, and active token count.
	// When nil, all metric writes are no-ops.
	metrics *Metrics

	// hmacKey is the secret used to derive the per-token storage key.
	// Set with [WithHMACKey] to share digests across processes; if unset, a
	// 32-byte random key is generated when [NewInMemoryStore] is invoked.
	hmacKey []byte `opt:"-"`
}
