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

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreio "github.com/altessa-s/go-atlas/core/io"
)

// maxUserInfoResponseSize is the maximum bytes read from the userinfo endpoint.
// A standard claims payload is well under 64 KiB; anything larger likely
// indicates a misbehaving or malicious upstream.
const maxUserInfoResponseSize = 1 << 16 // 64 KiB

// maxErrorBodyLen is the maximum number of bytes from an upstream error
// response body included in error messages, to avoid leaking PII or
// oversized payloads into logs and error chains.
const maxErrorBodyLen = 256

// sanitizeErrorBody truncates a raw response body to a safe length for
// inclusion in error messages.
func sanitizeErrorBody(b []byte) string {
	if len(b) <= maxErrorBodyLen {
		return string(b)
	}
	return string(b[:maxErrorBodyLen]) + "...(truncated)"
}

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
	endpoint := p.UserinfoEndpoint()
	if endpoint == "" {
		return nil, coreerrs.Wrap(ErrDiscovery, "userinfo endpoint is not available")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	token, err := tokenSource.Token()
	if err != nil {
		return nil, err
	}
	token.SetAuthHeader(req)

	resp, err := p.client.Do(req) //nolint:bodyclose
	if err != nil {
		return nil, err
	}
	defer drainAndClose(resp)

	buf := coreio.GetBuffer()
	_, err = buf.ReadFrom(io.LimitReader(resp.Body, maxUserInfoResponseSize))
	if err != nil {
		coreio.PutBuffer(buf)
		return nil, err
	}
	body := bytes.Clone(buf.Bytes())
	coreio.PutBuffer(buf)

	if resp.StatusCode != http.StatusOK {
		return nil, coreerrs.Wrapf(ErrTokenInvalid, "userinfo: unexpected status %s: %s", resp.Status, sanitizeErrorBody(body))
	}

	ct := resp.Header.Get("Content-Type")
	mediaType, _, parseErr := mime.ParseMediaType(ct)
	if parseErr == nil && mediaType == "application/jwt" {
		// Verify JWT signature and extract claims in a single operation
		claims, err := p.verifySignature(ctx, string(body))
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "verify userinfo JWT signature")
		}
		// Marshal claims back to JSON for UserInfo unmarshaling
		// This is still more efficient than double JWT parsing
		body, err = json.Marshal(claims)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "marshal claims")
		}
	}

	var userInfo UserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, coreerrs.WrapOperation(err, "decode userinfo")
	}

	return &userInfo, nil
}
