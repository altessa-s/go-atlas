// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"strings"

	"google.golang.org/grpc/credentials"
)

// TokenProvider is an interface for providing authentication tokens for outgoing requests.
// Implementations can fetch tokens from various sources (static, OAuth2, Vault, etc.).
type TokenProvider interface {
	// Token returns the authentication token to use for the request.
	// It receives the context which may contain request-specific information.
	// Returns empty string if no token should be attached.
	Token(ctx context.Context) (string, error)
}

// TokenProviderFunc is a function type that implements the TokenProvider interface.
type TokenProviderFunc func(ctx context.Context) (string, error)

// Token implements the TokenProvider interface.
func (f TokenProviderFunc) Token(ctx context.Context) (string, error) { return f(ctx) }

var _ TokenProvider = TokenProviderFunc(nil)

// StaticTokenProvider returns a TokenProvider that always returns the same token.
// Useful for API keys or long-lived tokens.
//
// Example:
//
//	provider := auth.StaticTokenProvider("my-api-key")
//	creds := auth.NewPerRPCCredentials(provider)
func StaticTokenProvider(token string) TokenProvider {
	return TokenProviderFunc(func(_ context.Context) (string, error) {
		return token, nil
	})
}

var _ credentials.PerRPCCredentials = (*perRPCCredentials)(nil)

type perRPCCredentials struct {
	provider   TokenProvider
	headerName string
	scheme     string
}

// NewPerRPCCredentials returns credentials that attach a token from provider
// to every outgoing gRPC call. They require transport security: gRPC refuses to
// send them over a plaintext connection, so a token never travels in cleartext
// by accident. For development without TLS use
// [github.com/altessa-s/go-atlas/transport/grpc/client.InsecureTokenCredentials].
//
// By default the token is sent as a Bearer token in the "authorization" header.
// Use WithHeaderName and WithScheme to customize the header and scheme.
//
// Example:
//
//	creds := auth.NewPerRPCCredentials(auth.TokenProviderFunc(func(ctx context.Context) (string, error) {
//	    return fetchTokenFromVault(ctx)
//	}))
//
//	c, err := client.New(ctx, target,
//	    client.WithDialOptions(grpc.WithPerRPCCredentials(creds)),
//	)
func NewPerRPCCredentials(provider TokenProvider, opts ...ClientOption) credentials.PerRPCCredentials {
	c := &perRPCCredentials{
		provider:   provider,
		headerName: "authorization",
		scheme:     "Bearer",
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// ClientOption configures the client token credentials.
type ClientOption func(*perRPCCredentials)

// WithHeaderName sets the metadata header name for the token.
// Default is "authorization".
func WithHeaderName(name string) ClientOption {
	return func(c *perRPCCredentials) {
		c.headerName = strings.ToLower(name)
	}
}

// WithScheme sets the authentication scheme prefix.
// Default is "Bearer". Set to empty string for no scheme prefix.
func WithScheme(scheme string) ClientOption {
	return func(c *perRPCCredentials) {
		c.scheme = scheme
	}
}

// RequireTransportSecurity reports true: the token is never sent over plaintext.
func (*perRPCCredentials) RequireTransportSecurity() bool { return true }

// GetRequestMetadata returns the token header for one call. It returns no
// metadata when the provider has no token to attach.
func (c *perRPCCredentials) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	token, err := c.provider.Token(ctx)
	if err != nil {
		return nil, err
	}

	if token == "" {
		return nil, nil //nolint:nilnil // No token means no metadata.
	}

	if c.scheme != "" {
		token = c.scheme + " " + token
	}
	return map[string]string{c.headerName: token}, nil
}
