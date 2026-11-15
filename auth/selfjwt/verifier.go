// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/altessa-s/go-atlas-core/runtime/singleflight"
	"github.com/altessa-s/go-atlas/auth/jwt"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
)

// Verifier validates self-issued tokens and returns the verified [Token]. It
// resolves the subject's public key by the token's kid through an in-process
// cache, backed by the key provider on a miss, and delegates the parse,
// signature, algorithm, and temporal/issuer/audience validation to
// [github.com/altessa-s/go-atlas/auth/jwt.Verifier]. Verification is fail-closed.
type Verifier struct {
	src   KeyProvider
	cache *keyCache
	jwt   *jwt.Verifier
	opts  *options
	// identityErr is non-nil when the verifier was built with an empty issuer
	// or audience; Verify then fails closed with it.
	identityErr error
	// flight collapses concurrent key-provider lookups for the same
	// (subject, kid) so a burst of misses for an uncached principal does not
	// stampede the provider, bounds each lookup by WithKeyLookupTimeout, and
	// caps distinct lookups in flight at WithMaxConcurrentLookups.
	flight *singleflight.Group[string, jwt.VerificationKey]
}

// NewVerifier builds a Verifier over the given key provider. A token must carry
// issuer as its iss claim and audience among its aud values; both are
// required, and [Verifier.Verify] fails with [ErrIssuerRequired] or
// [ErrAudienceRequired] when either is empty.
func NewVerifier(src KeyProvider, issuer, audience string, opts ...Option) *Verifier {
	o := newOptions(opts...)
	v := &Verifier{
		src:         src,
		cache:       newKeyCache(o.cacheTTL, o.negativeCacheTTL, o.cacheMaxEntries, o.clock),
		opts:        o,
		identityErr: checkIdentity(issuer, audience),
		flight: singleflight.New[string, jwt.VerificationKey](
			singleflight.WithTimeout(o.keyLookupTimeout),
			singleflight.WithMaxConcurrent(o.maxConcurrentLookups)),
	}
	jwtOpts := []jwt.Option{
		jwt.WithLeeway(o.leeway),
		jwt.WithClock(o.clock),
		jwt.WithAllowedAlgorithms(o.allowedAlgorithms...),
		jwt.WithIssuer(issuer),
		jwt.WithAudiences(audience),
	}
	jwtOpts = coreslices.AppendIf(jwtOpts, o.revocation != nil, jwt.WithRevocation(o.revocation))
	v.jwt = jwt.NewVerifier(jwt.KeyResolverFunc(v.resolveKey), jwtOpts...)
	return v
}

// Verify parses token (allowed algorithms only, expiration required), checks its
// signature against the subject's key for the token's kid, validates the
// temporal claims with the configured clock-skew leeway and the issuer and
// audience claims, and returns the verified [Token]. Errors map onto the
// package sentinels: [ErrTokenInvalid] for parse / algorithm / signature / iss /
// aud failures, [ErrTokenExpired] past exp, and
// the key-provider sentinels ([ErrSubjectUnknown], [ErrKeyRotated]) propagate
// unwrapped so a caller can translate them.
func (v *Verifier) Verify(ctx context.Context, raw string) (*Token, error) {
	start := time.Now()
	tok, err := v.verify(ctx, raw)
	v.opts.metrics.recordVerify(err == nil, time.Since(start))
	return tok, err
}

func (v *Verifier) verify(ctx context.Context, raw string) (*Token, error) {
	if v.identityErr != nil {
		return nil, v.identityErr
	}
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
	return vk, nil
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
	case errors.Is(err, jwt.ErrTokenRevoked):
		// Propagate the jwt sentinel unwrapped so a caller can errors.Is on it.
		return err
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
// propagate unwrapped so a caller can map them; [ErrSubjectUnknown] and
// [ErrKeyRotated] are also cached for WithNegativeCacheTTL, because subject
// and kid come from the token before its signature is verified.
func (v *Verifier) publicKey(ctx context.Context, subject, kid string) (jwt.VerificationKey, error) {
	if vk, ok := v.cache.get(subject, kid); ok {
		v.opts.metrics.recordCacheLookup(true)
		return vk, nil
	}
	if err := v.cache.getMiss(subject, kid); err != nil {
		v.opts.metrics.recordCacheLookup(true)
		return jwt.VerificationKey{}, err
	}
	v.opts.metrics.recordCacheLookup(false)

	// Collapse concurrent misses for the same (subject, kid) into a single
	// provider lookup. The flight key carries the invalidation generation read
	// before the lookup, so a verification that starts after InvalidateKey /
	// InvalidateSubject never joins a flight that may have read a retired key.
	// The lookup is detached from the winning caller's cancellation (it is
	// shared by every waiter) and bounded by the group's timeout; each caller
	// waits under its own context.
	gen := v.cache.generation()
	return v.flight.Do(ctx, flightKey(subject, kid, gen), func(lookupCtx context.Context) (jwt.VerificationKey, error) {
		// Re-check the cache: a peer flight for this key may have populated it
		// between our miss above and our entry into the singleflight group.
		if vk, ok := v.cache.get(subject, kid); ok {
			return vk, nil
		}
		if err := v.cache.getMiss(subject, kid); err != nil {
			return jwt.VerificationKey{}, err
		}
		vk, err := v.src.VerificationKey(lookupCtx, subject, kid)
		if err != nil {
			if errors.Is(err, ErrSubjectUnknown) || errors.Is(err, ErrKeyRotated) {
				v.cache.putMissIfGeneration(subject, kid, err, gen)
			}
			return jwt.VerificationKey{}, err
		}
		// Dropped when an invalidation happened while the provider was consulted.
		v.cache.putIfGeneration(subject, kid, vk, gen)
		return vk, nil
	})
}

// flightKey encodes (subject, kid, gen) injectively: the length prefixes make
// the subject/kid split unambiguous whatever bytes either contains, so two
// distinct pairs can never share a flight and thus a resolved key.
func flightKey(subject, kid string, gen uint64) string {
	b := make([]byte, 0, len(subject)+len(kid)+3*20)
	b = strconv.AppendInt(b, int64(len(subject)), 10)
	b = append(b, ':')
	b = append(b, subject...)
	b = strconv.AppendInt(b, int64(len(kid)), 10)
	b = append(b, ':')
	b = append(b, kid...)
	b = strconv.AppendUint(b, gen, 10)
	return string(b)
}

// InvalidateKey drops the cached verification key for (subject, kid), so the
// next verification of a token carrying that kid consults the [KeyProvider]
// again. Use it on key rotation to close the [WithCacheTTL] window during which
// a retired kid would otherwise keep verifying from cache. It also fences key
// lookups already in flight: their results are not cached, and verifications
// that start after the call do not share them. Only verifications that began
// before the call may still complete with the old key.
func (v *Verifier) InvalidateKey(subject, kid string) {
	v.cache.deleteKey(subject, kid)
}

// InvalidateSubject drops every cached verification key for subject, so the
// next verification for that subject reloads from the [KeyProvider]. Use it
// when a subject's keys rotate and the retired kid is not known to the caller.
// It fences in-flight key lookups exactly like [Verifier.InvalidateKey].
func (v *Verifier) InvalidateSubject(subject string) {
	v.cache.deleteSubject(subject)
}
