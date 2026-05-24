// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"errors"
	"net/http"
)

// Common authentication errors.
var (
	// ErrMissingToken is returned when no token is provided in the request.
	ErrMissingToken = errors.New("auth: missing token")

	// ErrInvalidToken is returned when the token format is invalid.
	ErrInvalidToken = errors.New("auth: invalid token")

	// ErrUnauthorized is returned when authentication fails.
	ErrUnauthorized = errors.New("auth: unauthorized")
)

// AuthFunc is the interface for authenticating tokens.
// It validates tokens and returns associated data or an error.
type AuthFunc interface {
	// Authenticate validates a token and returns associated data.
	// Returns an error if the token is invalid or authentication fails.
	Authenticate(ctx context.Context, token string) (any, error)
}

// AuthenticateFunc is a function type that implements the AuthFunc interface.
type AuthenticateFunc func(context.Context, string) (any, error)

// Authenticate implements the AuthFunc interface for AuthenticateFunc.
func (f AuthenticateFunc) Authenticate(ctx context.Context, token string) (any, error) {
	return f(ctx, token)
}

var _ AuthFunc = AuthenticateFunc(nil)

// TokenExtractor is the interface for extracting tokens from HTTP requests.
type TokenExtractor interface {
	// ExtractToken extracts a token from the HTTP request.
	// Returns an error if the token cannot be extracted or is missing.
	ExtractToken(r *http.Request) (string, error)
}

// TokenExtractorFunc is a function type that implements the TokenExtractor interface.
type TokenExtractorFunc func(*http.Request) (string, error)

// ExtractToken implements the TokenExtractor interface for TokenExtractorFunc.
func (f TokenExtractorFunc) ExtractToken(r *http.Request) (string, error) {
	return f(r)
}

var _ TokenExtractor = TokenExtractorFunc(nil)

// ErrorHandler is the interface for handling authentication errors.
type ErrorHandler interface {
	// HandleError writes an appropriate error response for authentication failures.
	HandleError(w http.ResponseWriter, r *http.Request, err error)
}

// ErrorHandlerFunc is a function type that implements the ErrorHandler interface.
type ErrorHandlerFunc func(http.ResponseWriter, *http.Request, error)

// HandleError implements the ErrorHandler interface for ErrorHandlerFunc.
func (f ErrorHandlerFunc) HandleError(w http.ResponseWriter, r *http.Request, err error) {
	f(w, r, err)
}

var _ ErrorHandler = ErrorHandlerFunc(nil)
