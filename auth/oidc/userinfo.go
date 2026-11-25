// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"

	"golang.org/x/oauth2"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coreio "github.com/altessa-s/go-atlas-core/io"
	authjwt "github.com/altessa-s/go-atlas/auth/jwt"
)

type userInfoClient struct {
	info   *discoveryInfo
	client *http.Client
	keys   *jwksKeySource
}

// maxUserInfoResponseSize is the maximum bytes read from the userinfo endpoint.
// A standard claims payload is well under 64 KiB; anything larger likely
// indicates a misbehaving or malicious upstream.
const maxUserInfoResponseSize = 1 << 16 // 64 KiB

// UserInfo represents user claims from the OIDC userinfo endpoint.
type UserInfo struct {
	Id            string   `json:"sub"`
	Email         string   `json:"email,omitempty"`
	Phone         string   `json:"phone_number,omitempty"`
	Name          string   `json:"given_name,omitempty"`
	LastName      string   `json:"family_name,omitempty"`
	CompanyId     string   `json:"company_id,omitempty"`
	CompanyName   string   `json:"company_name,omitempty"`
	Roles         []string `json:"roles,omitempty"`
	EmailVerified bool     `json:"email_verified,omitempty"`
	PhoneVerified bool     `json:"phone_number_verified,omitempty"`
}

// UserInfo retrieves user claims from the userinfo endpoint using the provided token.
// The token must have the "openid" scope.
//
// Example:
//
//	info, _ := provider.UserInfo(ctx, tokenSource)
func (p *Provider) UserInfo(ctx context.Context, tokenSource oauth2.TokenSource) (*UserInfo, error) {
	return p.userInfo.fetch(ctx, tokenSource)
}

func (p *userInfoClient) fetch(ctx context.Context, tokenSource oauth2.TokenSource) (*UserInfo, error) {
	if p.info == nil || p.info.UserInfoURL == "" {
		return nil, coreerrs.Wrap(ErrDiscovery, "userinfo endpoint is not available")
	}
	endpoint := p.info.UserInfoURL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, boundaryError("userinfo", 0, ErrTokenInvalid, err)
	}

	token, err := tokenSource.Token()
	if err != nil {
		return nil, boundaryError("userinfo", 0, ErrTokenInvalid, err)
	}
	token.SetAuthHeader(req)

	resp, err := p.client.Do(req) //nolint:bodyclose
	if err != nil {
		return nil, boundaryError("userinfo", 0, ErrTokenInvalid, err)
	}
	defer drainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, boundaryError("userinfo", resp.StatusCode, ErrTokenInvalid, nil)
	}

	buf := coreio.GetBuffer()
	_, err = buf.ReadFrom(io.LimitReader(resp.Body, maxUserInfoResponseSize))
	if err != nil {
		coreio.PutBuffer(buf)
		return nil, boundaryError("read userinfo", resp.StatusCode, ErrTokenInvalid, err)
	}
	body := bytes.Clone(buf.Bytes())
	coreio.PutBuffer(buf)

	ct := resp.Header.Get("Content-Type")
	mediaType, _, parseErr := mime.ParseMediaType(ct)
	if parseErr == nil && mediaType == "application/jwt" {
		if err := p.keys.checkJWKSStaleness(ctx); err != nil {
			return nil, err
		}
		// Verify JWT signature and extract claims in a single operation
		claims, err := p.verifySignature(ctx, string(body))
		if err != nil {
			return nil, boundaryError("verify userinfo JWT signature", resp.StatusCode, ErrTokenInvalid, err)
		}
		// Marshal claims back to JSON for UserInfo unmarshaling
		// This is still more efficient than double JWT parsing
		body, err = json.Marshal(claims)
		if err != nil {
			return nil, boundaryError("marshal claims", resp.StatusCode, ErrTokenInvalid, err)
		}
	}

	var userInfo UserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, boundaryError("decode userinfo", resp.StatusCode, ErrTokenInvalid, err)
	}

	return &userInfo, nil
}

// verifySignature verifies a JWT signature and validates its temporal claims
// when present, without binding audience / subject. It is used for signed
// userinfo responses, which legitimately omit exp — so exp is validated if
// present but not required (matching the prior signature-plus-temporal
// behavior). Leeway is zero to mirror the previous strict default. The issuer
// is bound when discovery resolved one, so a signed userinfo response from a
// different issuer sharing the JWKS is rejected.
func (p *userInfoClient) verifySignature(ctx context.Context, token string) (map[string]any, error) {
	opts := []authjwt.VerifierOption{
		authjwt.WithVerifierAllowedAlgorithms(toAuthAlgorithms(DefaultValidMethods)...),
		authjwt.WithExpirationOptional(),
		authjwt.WithLeeway(0),
	}
	opts = coreslices.AppendIfFunc(opts, p.info != nil && p.info.Issuer != "", func() []authjwt.VerifierOption {
		return []authjwt.VerifierOption{authjwt.WithVerifierIssuer(p.info.Issuer)}
	})
	v := authjwt.NewVerifier(p.keys.keyResolver, opts...)
	claims, err := v.Verify(ctx, token)
	if err != nil {
		return nil, err
	}
	return map[string]any(claims), nil
}
