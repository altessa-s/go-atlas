// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/altessa-s/go-atlas/auth/jwt"

	"golang.org/x/sync/singleflight"
)

// Verifier validates self-issued tokens and returns the verified [Token]. It
// resolves the subject's public key by the token's kid through an in-process
// cache, backed by the key provider on a miss, and delegates the parse,
// signature, algorithm, and temporal/issuer validation to
// [github.com/altessa-s/go-atlas/auth/jwt.Verifier]. Verification is fail-closed.
type Verifier struct {
	src   KeyProvider
	cache *keyCache
	jwt   *jwt.Verifier
	opts  *options
	// flight collapses concurrent key-provider lookups for the same
	// (subject, kid) so a burst of misses for an uncached principal does not
	// stampede the provider. Its zero value is ready for use.
	flight singleflight.Group
}

// NewVerifier builds a Verifier over the given key provider.
func NewVerifier(src KeyProvider, opts ...Option) *Verifier {
	o := newOptions(opts...)
	v := &Verifier{
		src:   src,
		cache: newKeyCache(o.cacheTTL, o.cacheMaxEntries, o.clock),
		opts:  o,
	}
	jwtOpts := []jwt.Option{
		jwt.WithLeeway(o.leeway),
		jwt.WithClock(o.clock),
		jwt.WithAllowedAlgorithms(toJWTAlgorithms(o.allowedAlgorithms)...),
	}
	if o.issuer != "" {
		jwtOpts = append(jwtOpts, jwt.WithIssuer(o.issuer))
	}
	v.jwt = jwt.NewVerifier(jwt.KeyResolverFunc(v.resolveKey), jwtOpts...)
	return v
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
	claims, err := v.jwt.Verify(ctx, raw)
	if err != nil {
		return nil, mapVerifyErr(err)
	}
	return &Token{
		Subject: claims.Subject(),
		ID:      claims.ID(),
		Scopes:  claims.Scopes(),
		Expiry:  claims.Expiry(),
	}, nil
}

// resolveKey is the [jwt.KeyResolver] seam: it maps a token's subject and kid to
// the subject's public key through the cache and key provider. The token's
// subject only locates the key; the signature is verified afterwards by the
// jwt verifier, which also enforces the key's algorithm binding.
func (v *Verifier) resolveKey(ctx context.Context, hdr jwt.Header, unverified jwt.Claims) (jwt.VerificationKey, error) {
	subject := unverified.Subject()
	if subject == "" || hdr.Kid == "" {
		return jwt.VerificationKey{}, ErrTokenInvalid
	}
	vk, err := v.publicKey(ctx, subject, hdr.Kid)
	if err != nil {
		return jwt.VerificationKey{}, err
	}
	// The resolved key must name the algorithm it verifies: binds each key to one
	// algorithm so a key can never be coerced into verifying a different scheme.
	if vk.Algorithm == "" {
		return jwt.VerificationKey{}, ErrAlgorithmNotAllowed
	}
	return jwt.VerificationKey{Algorithm: jwt.Algorithm(vk.Algorithm), Key: vk.Key}, nil
}

// mapVerifyErr translates auth/jwt verification errors back onto the selfjwt
// contract. Key-provider sentinels surfaced through the resolver propagate
// unwrapped; jwt's algorithm-binding and temporal sentinels are re-wrapped in
// the matching selfjwt sentinel.
func mapVerifyErr(err error) error {
	switch {
	case errors.Is(err, ErrSubjectUnknown), errors.Is(err, ErrKeyRotated):
		return err
	case errors.Is(err, ErrAlgorithmNotAllowed):
		return err
	case errors.Is(err, jwt.ErrAlgorithmNotAllowed):
		return fmt.Errorf("%w: %w", ErrAlgorithmNotAllowed, err)
	case errors.Is(err, jwt.ErrTokenExpired):
		return fmt.Errorf("%w: %w", ErrTokenExpired, err)
	case errors.Is(err, ErrTokenInvalid):
		return err
	default:
		return fmt.Errorf("%w: %w", ErrTokenInvalid, err)
	}
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
		// Detach the provider lookup from the winning caller's context:
		// singleflight shares one flight across all concurrent callers for this
		// (subject, kid), so honoring the winner's cancel/deadline here would
		// fail every waiter — including those whose own context is still live.
		// Value/trace propagation is preserved via WithoutCancel.
		vk, err := v.src.VerificationKey(context.WithoutCancel(ctx), subject, kid)
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

// InvalidateKey drops the cached verification key for (subject, kid), so the
// next verification of a token carrying that kid consults the [KeyProvider]
// again. Use it on key rotation to close the [WithCacheTTL] window during which
// a retired kid would otherwise keep verifying from cache. It is a no-op when
// nothing is cached for the pair.
func (v *Verifier) InvalidateKey(subject, kid string) {
	v.cache.deleteKey(subject, kid)
}

// InvalidateSubject drops every cached verification key for subject, so the
// next verification for that subject reloads from the [KeyProvider]. Use it
// when a subject's keys rotate and the retired kid is not known to the caller.
func (v *Verifier) InvalidateSubject(subject string) {
	v.cache.deleteSubject(subject)
}
