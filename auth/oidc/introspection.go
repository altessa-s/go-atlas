// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coreio "github.com/altessa-s/go-atlas-core/io"
	authjwt "github.com/altessa-s/go-atlas/auth/jwt"
)

const (
	// DefaultIntrospectionTimeout is the timeout for introspection HTTP requests.
	DefaultIntrospectionTimeout = 10 * time.Second

	// RevokedTokenCacheDuration is the TTL for caching revoked token status.
	RevokedTokenCacheDuration = 24 * time.Hour

	// ActiveTokenCacheDuration is the TTL for caching active introspection responses.
	ActiveTokenCacheDuration = 30 * time.Second

	// MaxErrorResponseSize is the maximum bytes to read from error responses.
	MaxErrorResponseSize = 2048

	// maxIntrospectionResponseSize is the maximum bytes read from a successful
	// introspection response, mirroring the userinfo cap. RFC 7662 responses
	// are small JSON objects; anything larger indicates a misbehaving or
	// malicious IdP.
	maxIntrospectionResponseSize = 1 << 16 // 64 KiB

	// RevocationItemTypeJTI identifies tokens by their JWT ID claim.
	RevocationItemTypeJTI = "jti"

	// RevocationItemTypeKID identifies tokens by their Key ID header.
	RevocationItemTypeKID = "kid"
)

// IntrospectionResponse represents an RFC 7662 token introspection response.
// Active is the only required field; others are optional metadata.
type IntrospectionResponse struct {
	Scope     string `json:"scope,omitempty"`
	ClientID  string `json:"client_id,omitempty"`
	Username  string `json:"username,omitempty"`
	TokenType string `json:"token_type,omitempty"`
	Sub       string `json:"sub,omitempty"`
	// Aud is the token audience. RFC 7662 mirrors JWT semantics, so aud may be a
	// single string or an array; authjwt.Audience accepts both on decode.
	Aud    authjwt.Audience `json:"aud,omitempty"`
	Iss    string           `json:"iss,omitempty"`
	Jti    string           `json:"jti,omitempty"`
	Exp    int64            `json:"exp,omitempty"`
	Iat    int64            `json:"iat,omitempty"`
	Nbf    int64            `json:"nbf,omitempty"`
	Active bool             `json:"active"`
}

// IntrospectToken performs RFC 7662 token introspection to check if a token is active.
// Requires introspection to be configured via WithIntrospection. Results are cached.
// The local revocation storage is not consulted here: token validation checks
// it separately (a local hit always rejects), and a local miss never stands in
// for the IdP's answer.
//
// Example:
//
//	resp, _ := provider.IntrospectToken(ctx, token)
//	if resp.Active { /* token is valid */ }
func (p *Provider) IntrospectToken(ctx context.Context, token string) (*IntrospectionResponse, error) {
	if p.discoveryInfo.IntrospectionURL == "" {
		return nil, coreerrs.Wrap(ErrIntrospection, "introspection endpoint not available in discovery document")
	}

	if p.opts.introspectionClientID == "" || p.opts.introspectionSecret.IsEmpty() {
		return nil, coreerrs.Wrap(ErrIntrospection, "introspection client credentials not configured")
	}

	if cached, ok := p.cachedIntrospection(ctx, token); ok {
		return cached, nil
	}

	introspectionResp, err := p.requestIntrospection(ctx, token)
	if err != nil {
		return nil, err
	}

	if p.tokenCache != nil {
		if introspectionResp.Active {
			p.cacheActive(ctx, token, introspectionResp)
		} else {
			p.cacheInactive(ctx, token, introspectionResp)
		}
	}

	return introspectionResp, nil
}

// cachedIntrospection returns the cached introspection result of token: a
// revoked token as inactive, or a previously confirmed active token's
// response.
func (p *Provider) cachedIntrospection(ctx context.Context, token string) (*IntrospectionResponse, bool) {
	if p.tokenCache == nil {
		return nil, false
	}

	// Check cache for revoked token first
	cacheKey := p.introspectionCacheKey(p.opts.revokedTokensCacheKeyPrefix, token)
	var cachedRevoked bool
	if err := p.tokenCache.Get(ctx, cacheKey, &cachedRevoked); err == nil && cachedRevoked {
		p.logger.DebugContext(ctx, "token found in revoked cache")
		return &IntrospectionResponse{Active: false}, true
	}

	// Check short-lived cache for previously confirmed active tokens
	cacheKey = p.introspectionCacheKey(p.opts.activeTokensCacheKeyPrefix, token)
	var cachedResponse IntrospectionResponse
	if err := p.tokenCache.Get(ctx, cacheKey, &cachedResponse); err == nil {
		p.logger.DebugContext(ctx, "token found in active cache")
		return &cachedResponse, true
	}
	return nil, false
}

// requestIntrospection asks the introspection endpoint about token.
func (p *Provider) requestIntrospection(ctx context.Context, token string) (*IntrospectionResponse, error) {
	// Prepare introspection request
	formData := url.Values{}
	formData.Set("token", token)
	formData.Set("token_type_hint", "access_token")

	requestCtx, cancel := corecontext.WithDefault(ctx, DefaultIntrospectionTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodPost,
		p.discoveryInfo.IntrospectionURL,
		strings.NewReader(formData.Encode()),
	)
	if err != nil {
		return nil, coreerrs.Wrapf(ErrIntrospection, "failed to create introspection request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(p.opts.introspectionClientID, p.opts.introspectionSecret.Expose())

	// Execute request
	resp, err := p.client.Do(req) //nolint:bodyclose
	if err != nil {
		return nil, coreerrs.Wrapf(ErrIntrospection, "introspection request failed: %v", err)
	}
	defer drainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, MaxErrorResponseSize)) //nolint:errcheck
		return nil, coreerrs.Wrapf(ErrIntrospection, "introspection endpoint returned status %d (%s)", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return decodeIntrospection(resp.Body)
}

// decodeIntrospection parses an introspection response body.
func decodeIntrospection(body io.Reader) (*IntrospectionResponse, error) {
	// Parse response using pooled buffer to avoid per-request allocation.
	// Security: cap the read — a malicious or misconfigured IdP must not be
	// able to OOM the validator with an unbounded response body.
	buf := coreio.GetBuffer()
	if _, err := buf.ReadFrom(io.LimitReader(body, maxIntrospectionResponseSize+1)); err != nil {
		coreio.PutBuffer(buf)
		return nil, coreerrs.Wrapf(ErrIntrospection, "failed to read introspection response: %v", err)
	}
	if buf.Len() > maxIntrospectionResponseSize {
		coreio.PutBuffer(buf)
		return nil, coreerrs.Wrapf(ErrIntrospection, "introspection response exceeds %d bytes", maxIntrospectionResponseSize)
	}

	var introspectionResp IntrospectionResponse
	unmarshalErr := json.Unmarshal(buf.Bytes(), &introspectionResp)
	coreio.PutBuffer(buf)
	if unmarshalErr != nil {
		return nil, coreerrs.Wrapf(ErrIntrospection, "failed to parse introspection response: %v", unmarshalErr)
	}
	return &introspectionResp, nil
}

// cacheActive caches an active introspection response, no longer than the
// token lives.
func (p *Provider) cacheActive(ctx context.Context, token string, introspectionResp *IntrospectionResponse) {
	ttl := boundedIntrospectionTTL(introspectionResp, ActiveTokenCacheDuration)
	if ttl > 0 {
		cacheKey := p.introspectionCacheKey(p.opts.activeTokensCacheKeyPrefix, token)
		_ = p.tokenCache.Save(ctx, cacheKey, *introspectionResp, ttl) //nolint:errcheck

		p.logger.DebugContext(ctx, "caching active token",
			"ttl", ttl)
	}
}

// cacheInactive caches an inactive token as revoked, no longer than the
// token lives, and adds it to the revocation storage.
func (p *Provider) cacheInactive(ctx context.Context, token string, introspectionResp *IntrospectionResponse) {
	ttl := boundedIntrospectionTTL(introspectionResp, RevokedTokenCacheDuration)
	if ttl > 0 {
		cacheKey := p.introspectionCacheKey(p.opts.revokedTokensCacheKeyPrefix, token)
		_ = p.tokenCache.Save(ctx, cacheKey, true, ttl) //nolint:errcheck

		p.logger.DebugContext(ctx, "caching revoked token",
			"ttl", ttl)
	}

	// Also add to revocation storage to optimize subsequent checks.
	if p.revocationStorage != nil {
		if item := p.revocationItem(token, introspectionResp); item != "" {
			_ = p.revocationStorage.MarkRevoked(ctx, item, ttl) //nolint:errcheck // best-effort
		}
	}
}

// revocationItem returns the item an inactive token is marked revoked by,
// or "" for none. We only populate the storage with values the IdP confirmed
// (full token, or jti returned by introspection). The kid is not returned by
// RFC 7662 introspection and reading it from the local header via
// ParseUnverified is unsafe — an attacker could craft a non-active token with
// the kid of a legitimate signing key, poisoning the revocation set and
// denying service to every token that genuinely uses that kid. Operators using
// kid-based revocation must populate it via the Sync path from a trusted
// source.
func (p *Provider) revocationItem(token string, introspectionResp *IntrospectionResponse) string {
	switch p.opts.revocationItemType {
	case RevocationItemTypeJTI:
		return introspectionResp.Jti
	case RevocationItemTypeKID:
		return "" // Skip — see security note above.
	default:
		return token
	}
}

// IntrospectionEndpoint returns the introspection URL from OIDC discovery.
// Returns empty string if introspection is not supported.
func (p *Provider) IntrospectionEndpoint() string {
	if p.discoveryInfo == nil {
		return ""
	}
	return p.discoveryInfo.IntrospectionURL
}

// boundedIntrospectionTTL calculates cache TTL bounded by token expiration time.
func boundedIntrospectionTTL(resp *IntrospectionResponse, maxTTL time.Duration) time.Duration {
	ttl := maxTTL
	if resp == nil {
		return ttl
	}

	if resp.Exp > 0 {
		remaining := time.Until(time.Unix(resp.Exp, 0))
		if remaining <= 0 {
			return 0
		}
		if remaining < ttl {
			ttl = remaining
		}
	}

	return ttl
}
