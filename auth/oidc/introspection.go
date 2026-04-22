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

	"github.com/golang-jwt/jwt/v5"

	"github.com/altessa-s/go-atlas/data/probfilter"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreio "github.com/altessa-s/go-atlas/core/io"
)

// Filter is an alias for probfilter.Filter
type Filter = probfilter.Filter

// DataLoader is an alias for probfilter.DataLoader
type DataLoader = probfilter.DataLoader

// RebuildableFilter is an alias for probfilter.RebuildableFilter
type RebuildableFilter = probfilter.RebuildableFilter

const (
	// DefaultIntrospectionTimeout is the timeout for introspection HTTP requests.
	DefaultIntrospectionTimeout = 10 * time.Second

	// RevokedTokenCacheDuration is the TTL for caching revoked token status.
	RevokedTokenCacheDuration = 24 * time.Hour

	// ActiveTokenCacheDuration is the TTL for caching active introspection responses.
	ActiveTokenCacheDuration = 30 * time.Second

	// MaxErrorResponseSize is the maximum bytes to read from error responses.
	MaxErrorResponseSize = 2048

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
	Aud       string `json:"aud,omitempty"`
	Iss       string `json:"iss,omitempty"`
	Jti       string `json:"jti,omitempty"`
	Exp       int64  `json:"exp,omitempty"`
	Iat       int64  `json:"iat,omitempty"`
	Nbf       int64  `json:"nbf,omitempty"`
	Active    bool   `json:"active"`
}

// IntrospectToken performs RFC 7662 token introspection to check if a token is active.
// Requires introspection to be configured via WithIntrospection. Results are cached.
//
// Example:
//
//	resp, _ := provider.IntrospectToken(ctx, token)
//	if resp.Active { /* token is valid */ }
func (p *Provider) IntrospectToken(ctx context.Context, token string) (*IntrospectionResponse, error) {
	if p.discoveryInfo.IntrospectionURL == "" {
		return nil, coreerrs.Wrap(ErrIntrospection, "introspection endpoint not available in discovery document")
	}

	if p.opts.introspectionClientID == "" || p.opts.introspectionSecret == "" {
		return nil, coreerrs.Wrap(ErrIntrospection, "introspection client credentials not configured")
	}

	// Check revocation storage first (pre-introspect check)
	if p.revocationStorage != nil {
		item := token
		switch p.opts.revocationItemType {
		case RevocationItemTypeJTI:
			if t, _, err := new(jwt.Parser).ParseUnverified(token, jwt.MapClaims{}); err == nil {
				if claims, ok := t.Claims.(jwt.MapClaims); ok {
					if jti, ok := claims[RevocationItemTypeJTI].(string); ok {
						item = jti
					}
				}
			}
		case RevocationItemTypeKID:
			if t, _, err := new(jwt.Parser).ParseUnverified(token, jwt.MapClaims{}); err == nil {
				if kid, ok := t.Header[RevocationItemTypeKID].(string); ok {
					item = kid
				}
			}
		}

		isRevoked, err := p.revocationStorage.IsRevoked(ctx, item)
		if err == nil && !isRevoked {
			// Storage (e.g. Bloom filter) says definitely NOT revoked
			p.logger.DebugContext(ctx, "item not found in revocation storage, skipping introspection",
				"item_type", p.opts.revocationItemType,
				"item", item)
			return &IntrospectionResponse{Active: true}, nil
		}
	}

	// Check cache for revoked token first
	if p.tokenCache != nil {
		cacheKey := tokenCacheKey(p.opts.revokedTokensCacheKeyPrefix, token)
		var cachedRevoked bool
		if err := p.tokenCache.Get(ctx, cacheKey, &cachedRevoked); err == nil && cachedRevoked {
			p.logger.DebugContext(ctx, "token found in revoked cache",
				"cache_key", cacheKey)
			return &IntrospectionResponse{Active: false}, nil
		}
	}

	// Check short-lived cache for previously confirmed active tokens
	if p.tokenCache != nil {
		cacheKey := tokenCacheKey(p.opts.activeTokensCacheKeyPrefix, token)
		var cachedResponse IntrospectionResponse
		if err := p.tokenCache.Get(ctx, cacheKey, &cachedResponse); err == nil {
			p.logger.DebugContext(ctx, "token found in active cache",
				"cache_key", cacheKey)
			return &cachedResponse, nil
		}
	}

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
	req.SetBasicAuth(p.opts.introspectionClientID, p.opts.introspectionSecret)

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

	// Parse response using pooled buffer to avoid per-request allocation.
	buf := coreio.GetBuffer()
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		coreio.PutBuffer(buf)
		return nil, coreerrs.Wrapf(ErrIntrospection, "failed to read introspection response: %v", err)
	}

	var introspectionResp IntrospectionResponse
	unmarshalErr := json.Unmarshal(buf.Bytes(), &introspectionResp)
	coreio.PutBuffer(buf)
	if unmarshalErr != nil {
		return nil, coreerrs.Wrapf(ErrIntrospection, "failed to parse introspection response: %v", unmarshalErr)
	}

	if p.tokenCache != nil {
		if !introspectionResp.Active {
			ttl := boundedIntrospectionTTL(&introspectionResp, RevokedTokenCacheDuration)
			if ttl > 0 {
				cacheKey := tokenCacheKey(p.opts.revokedTokensCacheKeyPrefix, token)
				_ = p.tokenCache.Save(ctx, cacheKey, true, ttl) //nolint:errcheck

				p.logger.DebugContext(ctx, "caching revoked token",
					"cache_key", cacheKey,
					"ttl", ttl)
			}

			// Also add to revocation storage to optimize subsequent checks
			if p.revocationStorage != nil {
				item := token
				if p.opts.revocationItemType == RevocationItemTypeJTI && introspectionResp.Jti != "" {
					item = introspectionResp.Jti
				} else if p.opts.revocationItemType == RevocationItemTypeKID {
					// KID is not typically returned in introspection response,
					// so we fall back to full token or use the one from header if we can parse it.
					if t, _, err := new(jwt.Parser).ParseUnverified(token, jwt.MapClaims{}); err == nil {
						if kid, ok := t.Header[RevocationItemTypeKID].(string); ok {
							item = kid
						}
					}
				}
				_ = p.revocationStorage.MarkRevoked(ctx, item, ttl) //nolint:errcheck // Best-effort operation
			}
		} else {
			ttl := boundedIntrospectionTTL(&introspectionResp, ActiveTokenCacheDuration)
			if ttl > 0 {
				cacheKey := tokenCacheKey(p.opts.activeTokensCacheKeyPrefix, token)
				_ = p.tokenCache.Save(ctx, cacheKey, introspectionResp, ttl) //nolint:errcheck

				p.logger.DebugContext(ctx, "caching active token",
					"cache_key", cacheKey,
					"ttl", ttl)
			}
		}
	}

	return &introspectionResp, nil
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
