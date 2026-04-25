// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"

	"golang.org/x/oauth2"
)

// StaticToken implements [credentials.PerRPCCredentials] by attaching a fixed
// Bearer token to every outgoing RPC. It requires TLS; for insecure transports
// use [InsecureTokenCredentials] instead.
type StaticToken struct {
	token string
}

// NewStaticToken creates a new static token credential.
func NewStaticToken(token string) StaticToken {
	return StaticToken{token: token}
}

// GetRequestMetadata returns the authorization header with a Bearer token.
func (t StaticToken) GetRequestMetadata(ctx context.Context, in ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + t.token,
	}, nil
}

// RequireTransportSecurity returns true to enforce TLS transport.
// Sending bearer tokens over plaintext connections risks credential interception.
// For development/testing without TLS, use InsecureTokenCredentials instead.
func (StaticToken) RequireTransportSecurity() bool {
	return true
}

// InsecureTokenCredentials implements [credentials.PerRPCCredentials] using an
// [oauth2.TokenSource] and explicitly disables the TLS requirement.
//
// WARNING: tokens are sent in cleartext. Use only in development or testing.
type InsecureTokenCredentials struct {
	tokenSource oauth2.TokenSource
}

// NewInsecureTokenCredentials creates a new insecure token credentials from an OAuth2 token source.
func NewInsecureTokenCredentials(tokenSource oauth2.TokenSource) InsecureTokenCredentials {
	return InsecureTokenCredentials{tokenSource: tokenSource}
}

// RequireTransportSecurity returns false to allow tokens over insecure connections.
// This is necessary for development environments but should never be used in production.
func (InsecureTokenCredentials) RequireTransportSecurity() bool {
	return false
}

// GetRequestMetadata fetches a fresh token from the underlying [oauth2.TokenSource]
// and returns it as a Bearer authorization header. Returns an error if the
// token source fails.
func (c InsecureTokenCredentials) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	token, err := c.tokenSource.Token()
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"authorization": "Bearer " + token.AccessToken,
	}, nil
}
