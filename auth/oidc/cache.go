// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"time"

	"github.com/golang-jwt/jwt/v5"

	authjwt "github.com/altessa-s/go-atlas/auth/jwt"
	corehash "github.com/altessa-s/go-atlas/core/encoding/hash"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

const (
	// DefaultTokensCacheKeyPrefix is the default prefix for validated token cache keys.
	DefaultTokensCacheKeyPrefix = "tokens:"

	// DefaultRevokedTokensCacheKeyPrefix is the default prefix for revoked token cache keys.
	DefaultRevokedTokensCacheKeyPrefix = "revoked-tokens:"

	// DefaultActiveTokensCacheKeyPrefix is the default prefix for active introspection cache keys.
	DefaultActiveTokensCacheKeyPrefix = "active-tokens:"
)

// Cacher defines the interface for caching validated tokens.
// Implementations must be thread-safe. Used to avoid redundant token
// signature verification: the provider caches only the signature-verified
// claims and header, and re-evaluates the requested validation policy
// (expiry, issuer, audience, scopes, CEL rules, …) and revocation on every
// cache hit.
//
// Example:
//
//	cache.Save(ctx, "key", claims, 5*time.Minute)
//	cache.Get(ctx, "key", &claims)
type Cacher interface {
	// Save stores a value in the cache with an optional TTL.
	Save(ctx context.Context, key string, value any, ttl ...time.Duration) error

	// Get retrieves a value from the cache by key into a pointer.
	Get(ctx context.Context, key string, value any) error
}

// tokenCacheKey generates a SHA-256 hash-based cache key from a token string.
// The prefix is prepended to the hex-encoded hash.
func tokenCacheKey(prefix, token string) string {
	return corehash.SHA256HexWithPrefix(prefix, token)
}

// signatureCacheVersion versions the signature-cache key derivation and entry
// format. Bumping it moves new entries to keys that older binaries never read
// (and vice versa), which keeps rolling upgrades that share a cache safe.
const signatureCacheVersion = "oidc-sig-v2"

// cachedToken is the signature-cache entry: the claims and header of a token
// whose signature this provider verified. It carries no policy decision.
type cachedToken struct {
	Claims map[string]any `json:"claims"`
	Header map[string]any `json:"header"`
	// KeyFingerprint identifies the public key that verified the signature
	// (SHA-256 of its PKIX encoding). A hit is honored only while the
	// provider's current JWKS still resolves the same key for the header, so
	// a key removed or replaced by a JWKS refresh invalidates the entry.
	KeyFingerprint string `json:"key_fingerprint"`
}

// introspectionCacheVersion versions the introspection-cache key derivation.
const introspectionCacheVersion = "oidc-introspect-v2"

// introspectionCacheKey derives an introspection-cache key (active or revoked
// namespace, selected by prefix) for token. It binds the introspection
// authority — issuer, introspection endpoint and client ID — so providers that
// share a cache never accept each other's introspection answers.
func (p *Provider) introspectionCacheKey(prefix, token string) string {
	var issuer, endpoint string
	if p.discoveryInfo != nil {
		issuer, endpoint = p.discoveryInfo.Issuer, p.discoveryInfo.IntrospectionURL
	}
	return tokenCacheKey(prefix, introspectionCacheVersion+"\x00"+issuer+"\x00"+endpoint+"\x00"+
		p.opts.introspectionClientID+"\x00"+token)
}

// signatureCacheKey derives the signature-cache key for token. Besides the
// token it binds the provider's trust domain (discovery issuer and JWKS URL),
// so providers that share a cache namespace but trust different keys never
// accept each other's signature verifications.
func (p *Provider) signatureCacheKey(token string) string {
	var issuer, jwksURL string
	if p.discoveryInfo != nil {
		issuer, jwksURL = p.discoveryInfo.Issuer, p.discoveryInfo.JwksURL
	}
	return tokenCacheKey(p.opts.tokensCacheKeyPrefix,
		signatureCacheVersion+"\x00"+issuer+"\x00"+jwksURL+"\x00"+token)
}

// verifyTokenSignature returns the signature-verified claims and header of
// token, enforcing the signing-algorithm allow-list of policy. It serves a previous verification from the token cache when one
// exists and otherwise verifies the signature against the JWKS, caching the
// result until the token's exp. Tokens without exp, or already past it, are
// never cached. Callers must still apply the claim policy to the result.
func (p *Provider) verifyTokenSignature(ctx context.Context, token string, policy *validationPolicy) (jwt.MapClaims, map[string]any, error) {
	ops := policy.ops
	if p.tokenCache == nil {
		verified, hdr, err := policy.signatureVerifier(p).VerifySignature(ctx, token)
		if err != nil {
			return nil, nil, err
		}
		return jwt.MapClaims(verified), headerToMap(hdr), nil
	}

	key := p.signatureCacheKey(token)
	var entry cachedToken
	if err := p.tokenCache.Get(ctx, key, &entry); err == nil && entry.Claims != nil && entry.KeyFingerprint != "" {
		alg, ok := entry.Header["alg"].(string)
		if ok && p.currentKeyFingerprint(ctx, entry.Header) == entry.KeyFingerprint {
			p.metrics.cacheHits.Inc()
			if !algorithmAllowed(ops, alg) {
				return nil, nil, coreerrs.Wrapf(ErrTokenInvalid, "signing algorithm %q not allowed", alg)
			}
			return jwt.MapClaims(entry.Claims), entry.Header, nil
		}
	}
	p.metrics.cacheMisses.Inc()

	// Record the exact key the signature verification used: a later,
	// independent lookup could already see a key rotated in between.
	var usedKey any
	resolver := authjwt.KeyResolverFunc(func(ctx context.Context, hdr authjwt.Header, c authjwt.Claims) (authjwt.VerificationKey, error) {
		vk, err := p.keyResolver.ResolveKey(ctx, hdr, c)
		usedKey = vk.Key
		return vk, err
	})
	v := authjwt.NewVerifier(resolver, p.jwtVerifyOptions(ops)...)
	verified, hdr, err := v.VerifySignature(ctx, token)
	if err != nil {
		return nil, nil, err
	}
	claims, header := jwt.MapClaims(verified), headerToMap(hdr)
	if ttl := getTokenExpirationTTL(claims); ttl > 0 {
		if fp := p.cachedKeyFingerprint(usedKey); fp != "" {
			entry := cachedToken{Claims: claims, Header: header, KeyFingerprint: fp}
			_ = p.tokenCache.Save(ctx, key, entry, ttl) //nolint:errcheck // best-effort cache
		}
	}
	return claims, header, nil
}

// currentKeyFingerprint resolves, through the provider's current JWKS, the
// key that verifies tokens with header and returns its fingerprint, or "" if
// no key resolves or it cannot be encoded.
func (p *Provider) currentKeyFingerprint(ctx context.Context, header map[string]any) string {
	alg, _ := header["alg"].(string)
	kid, _ := header["kid"].(string)
	vk, err := p.keyResolver.ResolveKey(ctx, authjwt.Header{Alg: alg, Kid: kid}, nil)
	if err != nil {
		return ""
	}
	return p.cachedKeyFingerprint(vk.Key)
}

// keyFingerprintMemo is one memoized key fingerprint.
type keyFingerprintMemo struct {
	key any
	fp  string
}

// cachedKeyFingerprint returns keyFingerprint(key), memoizing the result for
// the most recent pointer-typed key: the JWKS hands out the same key object
// until a refresh replaces it, so a cache hit does not re-encode it. Only
// pointer keys are memoized — comparing them is always safe and the memo keeps
// the key alive, so its identity cannot be reused by another key; other key
// types (e.g. ed25519.PublicKey, a slice) are computed every time.
func (p *Provider) cachedKeyFingerprint(key any) string {
	switch key.(type) {
	case *rsa.PublicKey, *ecdsa.PublicKey:
	default:
		return keyFingerprint(key)
	}
	if m := p.lastKeyFingerprint.Load(); m != nil && m.key == key {
		return m.fp
	}
	fp := keyFingerprint(key)
	if fp != "" {
		p.lastKeyFingerprint.Store(&keyFingerprintMemo{key: key, fp: fp})
	}
	return fp
}

// keyFingerprint returns the hex SHA-256 of key's PKIX encoding, or "" for a
// nil or unencodable key.
func keyFingerprint(key any) string {
	if key == nil {
		return ""
	}
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}
