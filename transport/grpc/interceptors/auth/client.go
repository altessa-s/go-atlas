// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"fmt"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
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
	provider TokenProvider
	opts     clientOptions
}

// NewPerRPCCredentials returns credentials that attach a token from provider
// to every outgoing gRPC call. They require transport security: gRPC refuses to
// send them over a plaintext connection, so a token never travels in cleartext
// by accident. For development without TLS use
// [github.com/altessa-s/go-atlas/transport/grpc/client.InsecureTokenCredentials].
//
// By default the token is sent as a Bearer token in the "authorization" header.
// Use WithHeaderName, WithScheme and WithNoScheme to customize the header and scheme.
//
// A call whose outgoing metadata already carries the credential header is
// rejected with codes.Unauthenticated instead of sending two values, and a
// provider failure is reported with a fixed message: the provider's error text
// never reaches the call status. A nil provider is a construction error that
// matches [coreerrs.ErrInvalidArgument].
//
// Example:
//
//	creds, err := auth.NewPerRPCCredentials(auth.TokenProviderFunc(func(ctx context.Context) (string, error) {
//	    return fetchTokenFromVault(ctx)
//	}))
//	if err != nil {
//	    return err
//	}
//	c, err := client.New(ctx, target,
//	    client.WithDialOptions(grpc.WithPerRPCCredentials(creds)),
//	)
func NewPerRPCCredentials(provider TokenProvider, opts ...ClientOption) (credentials.PerRPCCredentials, error) {
	if nilcheck.IsNil(provider) {
		return nil, fmt.Errorf("%w: token provider is required", coreerrs.ErrInvalidArgument)
	}
	c := &perRPCCredentials{provider: provider, opts: *newClientOptions(opts...)}
	return c, nil
}

// RequireTransportSecurity reports true: the token is never sent over plaintext.
func (*perRPCCredentials) RequireTransportSecurity() bool { return true }

// GetRequestMetadata returns the token header for one call. It returns no
// metadata when the provider has no token to attach.
func (c *perRPCCredentials) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	if md, ok := metadata.FromOutgoingContext(ctx); ok && len(md.Get(c.opts.headerName)) > 0 {
		return nil, status.Error(codes.Unauthenticated, "credential header already set on the call")
	}

	token, err := c.provider.Token(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "token unavailable")
	}

	if token == "" {
		return nil, nil //nolint:nilnil // No token means no metadata.
	}

	if !c.opts.noScheme {
		token = c.opts.scheme + " " + token
	}
	return map[string]string{c.opts.headerName: token}, nil
}
