// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

//go:generate go tool optgen generate --type=minterOptions --option-type=MinterOption --output=minter_options_gen.go
//go:generate go tool optgen generate --type=verifierOptions --option-type=VerifierOption --output=verifier_options_gen.go
//go:generate go tool optgen generate --type=pairOptions --option-type=PairOption --output=pair_options_gen.go --option-prefix=Pair

import (
	"crypto/rand"
	"io"
	"time"

	"github.com/altessa-s/go-atlas/auth/jwt"
)

// Default tunables for the minter and verifier.
const (
	// DefaultMaxTokenLifetime is the ceiling a requested token TTL is clamped to.
	DefaultMaxTokenLifetime = 30 * 24 * time.Hour

	// DefaultLeeway is the clock-skew tolerance applied to exp / nbf verification
	// to absorb clock drift between minter and verifier. Named to match
	// [github.com/altessa-s/go-atlas/auth/jwt.DefaultLeeway].
	DefaultLeeway = 30 * time.Second

	// DefaultCacheTTL is how long a resolved verification key is cached in
	// process before it is reloaded from the key provider.
	DefaultCacheTTL = 5 * time.Minute

	// DefaultCacheMaxEntries caps the verification-key cache so a churn of
	// short-lived subjects or rotated-away kids cannot grow it without bound.
	// A tenth of it is the separate cap of cached key-provider misses, so
	// unknown subjects cannot evict verified keys. A non-positive
	// WithCacheMaxEntries selects this default.
	DefaultCacheMaxEntries = 10000

	// DefaultKeyLookupTimeout bounds a key-provider lookup on a cache miss. The
	// lookup is shared by concurrent verifications of the same (subject, kid)
	// and detached from any single caller's cancellation, so it needs its own
	// deadline.
	DefaultKeyLookupTimeout = 10 * time.Second

	// DefaultNegativeCacheTTL is how long a key-provider miss
	// ([ErrSubjectUnknown], [ErrKeyRotated]) for a (subject, kid) is
	// remembered, so tokens carrying unknown subjects or kids cannot drive a
	// provider lookup per request.
	DefaultNegativeCacheTTL = 30 * time.Second

	// DefaultMaxConcurrentLookups caps the distinct (subject, kid) key-provider
	// lookups in flight; a lookup for a new pair beyond it fails fast with
	// [github.com/altessa-s/go-atlas-core/runtime/singleflight.ErrOverloaded].
	DefaultMaxConcurrentLookups = 64
)

// defaultRand is the production randomness source for jti generation.
var defaultRand io.Reader = rand.Reader

type minterOptions struct {
	maxTokenLifetime time.Duration `optgen:"default=DefaultMaxTokenLifetime"`
	// WithMinterAllowedAlgorithms replaces the minter's algorithm allow-list.
	// An empty list keeps the current algorithms.
	allowedAlgorithms []jwt.Algorithm `opt:"MinterAllowedAlgorithms" optval:"clone,notrim,nodup" optgen:"default=defaultAllowedAlgorithms"`
	rand              io.Reader       `optgen:"default=defaultRand"`
	clock             Clock           `opt:"MinterClock" optgen:"default=defaultClock"`
	// metrics records mint/verify outcomes, latency, and cache hit/miss.
	// When nil, all metric writes are no-ops.
	metrics *Metrics `opt:"MinterMetrics"`
}

type verifierOptions struct {
	leeway           time.Duration `optgen:"default=DefaultLeeway" optval:"positive=allow_zero"`
	cacheTTL         time.Duration `optgen:"default=DefaultCacheTTL" optval:"positive=allow_zero"`
	cacheMaxEntries  int           `optgen:"default=DefaultCacheMaxEntries"`
	keyLookupTimeout time.Duration `optgen:"default=DefaultKeyLookupTimeout" optval:"positive"`
	negativeCacheTTL time.Duration `optgen:"default=DefaultNegativeCacheTTL" optval:"positive=allow_zero"`
	// maxConcurrentLookups bounds the distinct key-provider lookups in flight.
	maxConcurrentLookups int `optgen:"default=DefaultMaxConcurrentLookups" optval:"positive"`
	// WithVerifierAllowedAlgorithms replaces the verifier's algorithm allow-list.
	// An empty list keeps the current algorithms.
	allowedAlgorithms []jwt.Algorithm `opt:"VerifierAllowedAlgorithms" optval:"clone,notrim,nodup" optgen:"default=defaultAllowedAlgorithms"`
	clock             Clock           `opt:"VerifierClock" optgen:"default=defaultClock"`
	// metrics records mint/verify outcomes, latency, and cache hit/miss.
	// When nil, all metric writes are no-ops.
	metrics *Metrics `opt:"VerifierMetrics"`
	// revocation, when set, rejects a verified token whose jti has been revoked.
	// It is forwarded to the embedded jwt verifier and reported as
	// [github.com/altessa-s/go-atlas/auth/jwt.ErrTokenRevoked]. Nil leaves
	// revocation unchecked. Set via the generated WithRevocation.
	revocation jwt.RevocationChecker `optgen:"notnil"`
}

type pairOptions struct {
	maxTokenLifetime     time.Duration   `optgen:"default=DefaultMaxTokenLifetime"`
	leeway               time.Duration   `optgen:"default=DefaultLeeway" optval:"positive=allow_zero"`
	cacheTTL             time.Duration   `optgen:"default=DefaultCacheTTL" optval:"positive=allow_zero"`
	cacheMaxEntries      int             `optgen:"default=DefaultCacheMaxEntries"`
	keyLookupTimeout     time.Duration   `optgen:"default=DefaultKeyLookupTimeout" optval:"positive"`
	negativeCacheTTL     time.Duration   `optgen:"default=DefaultNegativeCacheTTL" optval:"positive=allow_zero"`
	maxConcurrentLookups int             `optgen:"default=DefaultMaxConcurrentLookups" optval:"positive"`
	allowedAlgorithms    []jwt.Algorithm `opt:"AllowedAlgorithms" optval:"clone,notrim,nodup" optgen:"default=defaultAllowedAlgorithms"`
	rand                 io.Reader       `optgen:"default=defaultRand"`
	clock                Clock           `optgen:"default=defaultClock"`
	metrics              *Metrics
	revocation           jwt.RevocationChecker `optgen:"notnil"`
}
