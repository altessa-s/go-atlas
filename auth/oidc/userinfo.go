// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"

	"golang.org/x/oauth2"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// UserInfo represents user claims from the OIDC userinfo endpoint.
type UserInfo struct {
	Id            string   `json:"sub"`
	Email         string   `json:"email,omitempty"`
	EmailVerified bool     `json:"email_verified,omitempty"`
	Phone         string   `json:"phone_number,omitempty"`
	PhoneVerified bool     `json:"phone_number_verified,omitempty"`
	Name          string   `json:"given_name,omitempty"`
	LastName      string   `json:"family_name,omitempty"`
	CompanyId     string   `json:"company_id,omitempty"`
	CompanyName   string   `json:"company_name,omitempty"`
	Roles         []string `json:"roles,omitempty"`
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
		return nil, fmt.Errorf("userinfo endpoint is not available")
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s: %s", resp.Status, body)
	}

	ct := resp.Header.Get("Content-Type")
	mediaType, _, parseErr := mime.ParseMediaType(ct)
	if parseErr == nil && mediaType == "application/jwt" {
		// Verify JWT signature and extract claims in a single operation
		claims, err := p.verifySignature(string(body))
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
