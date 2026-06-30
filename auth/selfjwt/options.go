// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"crypto/rand"
	"io"
	"slices"
	"time"

	"github.com/altessa-s/go-atlas/auth/jwt"
)

// Default tunables for the minter and verifier.
const (
	// DefaultIssuer is the empty iss claim; deployments override it with
	// [WithIssuer].
	DefaultIssuer = ""

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
	DefaultCacheMaxEntries = 10000
)

// defaultRand is the production randomness source for jti generation.
var defaultRand io.Reader = rand.Reader

// options carries the minter/verifier tunables. rand and clock are injectable
// seams for deterministic tests.
type options struct {
	issuer            string        `optgen:"default=DefaultIssuer"`
	maxTokenLifetime  time.Duration `optgen:"default=DefaultMaxTokenLifetime"`
	leeway            time.Duration `optgen:"default=DefaultLeeway" optval:"positive=allow_zero"`
	cacheTTL          time.Duration `optgen:"default=DefaultCacheTTL" optval:"positive=allow_zero"`
	cacheMaxEntries   int           `optgen:"default=DefaultCacheMaxEntries"`
	allowedAlgorithms []Algorithm   `optgen:"manual,default=defaultAllowedAlgorithms"`
	rand              io.Reader     `optgen:"default=defaultRand"`
	clock             Clock         `optgen:"default=defaultClock"`

	// metrics records mint/verify outcomes, latency, and cache hit/miss.
	// When nil, all metric writes are no-ops.
	metrics *Metrics

	// revocation, when set, rejects a verified token whose jti has been revoked.
	// It is forwarded to the embedded jwt verifier and reported as
	// [github.com/altessa-s/go-atlas/auth/jwt.ErrTokenRevoked]. Nil leaves
	// revocation unchecked. Set via the generated WithRevocation.
	revocation jwt.RevocationChecker `optgen:"notnil"`
}

// WithAllowedAlgorithms sets the signature algorithms the verifier accepts,
// replacing the default asymmetric allow-list ([DefaultAllowedAlgorithms]).
// Restricting or extending the set is the supported way to add support for a
// new algorithm: register a signing method with golang-jwt, then list it here.
// A call with no algorithms is ignored so the safe default is never cleared.
func WithAllowedAlgorithms(algs ...Algorithm) Option {
	return func(o *options) {
		if len(algs) == 0 {
			return
		}
		o.allowedAlgorithms = slices.Clone(algs)
	}
}
