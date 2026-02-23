// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc/metadata"

	stdGrpc "google.golang.org/grpc"
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
//	interceptor := auth.ClientInterceptor(provider)
func StaticTokenProvider(token string) TokenProvider {
	return TokenProviderFunc(func(_ context.Context) (string, error) {
		return token, nil
	})
}

var _ interceptors.ClientInterceptor = (*clientInterceptor)(nil)

type clientInterceptor struct {
	provider   TokenProvider
	headerName string
	scheme     string
}

// ClientInterceptor returns a new client interceptor that attaches authentication tokens
// to outgoing gRPC requests. The token is obtained from the provided TokenProvider and
// attached to the request metadata.
//
// By default, tokens are attached as Bearer tokens in the "authorization" header.
// Use WithHeaderName and WithScheme options to customize the header and scheme.
//
// Example:
//
//	// Using static token
//	interceptor := auth.ClientInterceptor(auth.StaticTokenProvider("my-token"))
//
//	// Using dynamic token provider
//	interceptor := auth.ClientInterceptor(auth.TokenProviderFunc(func(ctx context.Context) (string, error) {
//	    return fetchTokenFromVault(ctx)
//	}))
//
//	conn, err := grpc.Dial(target,
//	    grpc.WithUnaryInterceptor(interceptor.ClientUnaryInterceptor()),
//	    grpc.WithStreamInterceptor(interceptor.ClientStreamInterceptor()),
//	)
func ClientInterceptor(provider TokenProvider, opts ...ClientOption) interceptors.ClientInterceptor {
	ic := &clientInterceptor{
		provider:   provider,
		headerName: "authorization",
		scheme:     "Bearer",
	}

	for _, opt := range opts {
		opt(ic)
	}

	return ic
}

// ClientOption configures the client auth interceptor.
type ClientOption func(*clientInterceptor)

// WithHeaderName sets the metadata header name for the token.
// Default is "authorization".
func WithHeaderName(name string) ClientOption {
	return func(c *clientInterceptor) {
		c.headerName = name
	}
}

// WithScheme sets the authentication scheme prefix.
// Default is "Bearer". Set to empty string for no scheme prefix.
func WithScheme(scheme string) ClientOption {
	return func(c *clientInterceptor) {
		c.scheme = scheme
	}
}

// Name returns the interceptor name.
func (c *clientInterceptor) Name() string {
	return interceptorName
}

// ClientUnaryInterceptor returns a unary client interceptor.
func (c *clientInterceptor) ClientUnaryInterceptor() stdGrpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *stdGrpc.ClientConn,
		invoker stdGrpc.UnaryInvoker,
		opts ...stdGrpc.CallOption,
	) error {
		ctx, err := c.attachToken(ctx)
		if err != nil {
			return err
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// ClientStreamInterceptor returns a streaming client interceptor.
func (c *clientInterceptor) ClientStreamInterceptor() stdGrpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *stdGrpc.StreamDesc,
		cc *stdGrpc.ClientConn,
		method string,
		streamer stdGrpc.Streamer,
		opts ...stdGrpc.CallOption,
	) (stdGrpc.ClientStream, error) {
		ctx, err := c.attachToken(ctx)
		if err != nil {
			return nil, err
		}
		return streamer(ctx, desc, cc, method, opts...)
	}
}

// attachToken gets the token from the provider and attaches it to the context metadata.
func (c *clientInterceptor) attachToken(ctx context.Context) (context.Context, error) {
	token, err := c.provider.Token(ctx)
	if err != nil {
		return ctx, err
	}

	if token == "" {
		return ctx, nil
	}

	// Build the header value
	headerValue := token
	if c.scheme != "" {
		headerValue = c.scheme + " " + token
	}

	// Get existing metadata or create new
	md, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = md.Copy()
	} else {
		md = metadata.New(nil)
	}

	md.Set(c.headerName, headerValue)
	return metadata.NewOutgoingContext(ctx, md), nil
}
