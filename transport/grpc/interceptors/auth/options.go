// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

//go:generate go tool optgen generate --type=options

import (
	"context"
	"log/slog"
	"regexp"

	"google.golang.org/grpc/codes"
)

// Auth is an interface for authenticating requests.
// It validates authentication credentials and returns associated data or an error.
type Auth interface {
	Auth(ctx context.Context, ac Request) (any, error)
}

// AuthFunc is a function type that implements the Auth interface.
type AuthFunc func(context.Context, Request) (any, error)

// Auth implements the Auth interface for AuthFunc.
func (f AuthFunc) Auth(ctx context.Context, ac Request) (any, error) { return f(ctx, ac) }

var _ Auth = AuthFunc(nil)

// ClientAuth is an interface for authenticating the client.
// It performs additional client-level authentication checks after initial auth.
type ClientAuth interface {
	ClientAuth(ctx context.Context, cred Credentials) (context.Context, error)
}

// ClientAuthFunc is a function type that implements the ClientAuth interface.
type ClientAuthFunc func(ctx context.Context, cred Credentials) (context.Context, error)

// ClientAuth implements the ClientAuth interface for ClientAuthFunc.
func (f ClientAuthFunc) ClientAuth(ctx context.Context, cred Credentials) (context.Context, error) {
	return f(ctx, cred)
}

var _ ClientAuth = ClientAuthFunc(nil)

// Rejection is a deliberate refusal a callback may publish. The client receives Code with the code's name as the
// message and an ErrorInfo detail carrying Reason and Domain. A Code outside Canceled..Unauthenticated becomes
// Unauthenticated, a Reason that is not an AIP-193 reason (^[A-Z][A-Z0-9_]{1,61}[A-Z0-9]$) is dropped, and so is a
// Domain that is not a lowercase host name. A nil *Rejection is a plain Unauthenticated refusal.
type Rejection struct {
	Code   codes.Code
	Reason string
	Domain string
}

// Error returns the code and the reason. A nil Rejection reads as Unauthenticated.
func (r *Rejection) Error() string {
	if r == nil {
		return codes.Unauthenticated.String()
	}
	return r.Code.String() + ": " + r.Reason
}

// TokenExtractor is an interface for extracting authentication tokens from context.
// It abstracts the token extraction logic, allowing different token sources (headers, metadata, etc.).
type TokenExtractor interface {
	ExtractToken(ctx context.Context) (string, error)
}

// TokenExtractorFunc is a function type that implements the TokenExtractor interface.
type TokenExtractorFunc func(ctx context.Context) (string, error)

// ExtractToken implements the TokenExtractor interface for TokenExtractorFunc.
func (f TokenExtractorFunc) ExtractToken(ctx context.Context) (string, error) { return f(ctx) }

var _ TokenExtractor = TokenExtractorFunc(nil)

// options holds configuration for the auth interceptor.
type options struct {
	authFn         Auth
	logger         *slog.Logger
	tokenExtractor TokenExtractor
	// ignoreMethods and ignorePatterns have no defaults: authentication
	// exempts nothing unless the caller configures it.
	ignoreMethods  []string
	ignorePatterns []*regexp.Regexp
	clientAuth     ClientAuth
}
