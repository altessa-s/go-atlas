// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"golang.org/x/sync/singleflight"
)

// Verifier validates self-issued tokens and returns the verified [Token]. It
// resolves the subject's public key by the token's kid through an in-process
// cache, backed by the key provider on a miss. Verification is fail-closed.
type Verifier struct {
	src   KeyProvider
	cache *keyCache
	opts  *options
	// parserOpts are the JWT parser options, fixed for the verifier's lifetime
	// and built once in NewVerifier so Verify allocates no option slice per call.
	parserOpts []jwt.ParserOption
	// flight collapses concurrent key-provider lookups for the same
	// (subject, kid) so a burst of misses for an uncached principal does not
	// stampede the provider. Its zero value is ready for use.
	flight singleflight.Group
}

// NewVerifier builds a Verifier over the given key provider.
func NewVerifier(src KeyProvider, opts ...Option) *Verifier {
	o := newOptions(opts...)
	methods := make([]string, len(o.allowedAlgorithms))
	for i, a := range o.allowedAlgorithms {
		methods[i] = a.String()
	}
	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods(methods),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(o.clockSkew),
		jwt.WithTimeFunc(o.clock.Now),
	}
	if o.issuer != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(o.issuer))
	}
	return &Verifier{
		src:        src,
		cache:      newKeyCache(o.cacheTTL, o.cacheMaxEntries, o.clock),
		opts:       o,
		parserOpts: parserOpts,
	}
}

// Verify parses token (allowed algorithms only, expiration required), checks its
// signature against the subject's key for the token's kid, validates the
// temporal and issuer claims with the configured clock-skew leeway, and returns
// the verified [Token]. Errors map onto the package sentinels: [ErrTokenInvalid]
// for parse / algorithm / signature failures, [ErrTokenExpired] past exp, and
// the key-provider sentinels ([ErrSubjectUnknown], [ErrKeyRotated]) propagate
// unwrapped so a caller can translate them.
func (v *Verifier) Verify(ctx context.Context, raw string) (*Token, error) {
	start := time.Now()
	tok, err := v.verify(ctx, raw)
	v.opts.metrics.recordVerify(err == nil, time.Since(start))
	return tok, err
}

func (v *Verifier) verify(ctx context.Context, raw string) (*Token, error) {
	// keyErr captures a key-resolution failure raised inside the key func so the
	// precise sentinel survives golang-jwt's error wrapping.
	var claims tokenClaims
	var keyErr error
	_, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		// claims is populated (still unverified) before the key func runs; the
		// subject only locates the key, the signature is verified afterwards.
		kid, _ := t.Header["kid"].(string)
		if claims.Subject == "" || kid == "" {
			keyErr = ErrTokenInvalid
			return nil, keyErr
		}
		vk, e := v.publicKey(ctx, claims.Subject, kid)
		if e != nil {
			keyErr = e
			return nil, e
		}
		// The resolved key must name the algorithm it verifies and that algorithm
		// must match the token header: binds each key to one algorithm so a key
		// can never be coerced into verifying a different scheme.
		if vk.Algorithm == "" || vk.Algorithm.String() != t.Method.Alg() {
			keyErr = ErrAlgorithmNotAllowed
			return nil, keyErr
		}
		return vk.Key, nil
	}, v.parserOpts...)
	if err != nil {
		if keyErr != nil {
			return nil, keyErr
		}
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, fmt.Errorf("%w: %w", ErrTokenExpired, err)
		}
		return nil, fmt.Errorf("%w: %w", ErrTokenInvalid, err)
	}

	return &Token{
		Subject: claims.Subject,
		ID:      claims.ID,
		Scopes:  claims.Scopes,
		Expiry:  claims.ExpiresAt.Time,
	}, nil
}

// publicKey resolves the subject's verification key for kid, consulting the
// cache first and the key provider on a miss. The key-provider sentinels
// propagate unwrapped so a caller can map them.
func (v *Verifier) publicKey(ctx context.Context, subject, kid string) (VerificationKey, error) {
	if vk, ok := v.cache.get(subject, kid); ok {
		v.opts.metrics.recordCacheLookup(true)
		return vk, nil
	}
	v.opts.metrics.recordCacheLookup(false)

	// Collapse concurrent misses for the same (subject, kid) into a single
	// provider lookup. The NUL separator keeps the flight key unambiguous
	// across the subject/kid boundary.
	res, err, _ := v.flight.Do(subject+"\x00"+kid, func() (any, error) {
		// Re-check the cache: a peer flight for this key may have populated it
		// between our miss above and our entry into the singleflight group.
		if vk, ok := v.cache.get(subject, kid); ok {
			return vk, nil
		}
		vk, err := v.src.VerificationKey(ctx, subject, kid)
		if err != nil {
			return VerificationKey{}, err
		}
		v.cache.put(subject, kid, vk)
		return vk, nil
	})
	if err != nil {
		return VerificationKey{}, err
	}
	vk, _ := res.(VerificationKey)
	return vk, nil
}
