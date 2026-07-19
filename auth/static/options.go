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

// MinHMACKeyBytes is the smallest HMAC key accepted by [WithHMACKey]. Shorter
// values are silently ignored. NIST SP 800-107 recommends keys at least as
// long as the digest size — for HMAC-SHA256 that is 32 bytes.
const MinHMACKeyBytes = 16

// WithHMACKey overrides the per-instance random HMAC key with a caller-supplied
// one. Provide a stable key when multiple processes must agree on the storage
// digest of a token (for example, for shared-cache or rate-limit keying).
//
// Keys shorter than [MinHMACKeyBytes] are ignored and the default random key
// is used instead.
func WithHMACKey(key []byte) Option {
	return func(o *options) {
		if len(key) < MinHMACKeyBytes {
			return
		}
		dup := make([]byte, len(key))
		copy(dup, key)
		o.hmacKey = dup
	}
}
