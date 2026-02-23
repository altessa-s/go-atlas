// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Validator defines the interface for OIDC token validation.
type Validator interface {
	// ValidateToken verifies the token and extracts claims from it.
	// Returns an error if the token is invalid, expired, or verification fails.
	ValidateToken(ctx context.Context, token string) (map[string]any, error)
}

// AuthFunc creates a gRPC authentication function that validates OIDC tokens.
// This function integrates with the grpc/interceptors/auth package to provide
// OIDC-based authentication for gRPC services.
//
// The returned auth.Func performs the following steps:
//  1. Extracts the token from the request metadata (expects Bearer token format)
//  2. Validates the token using the provided Validator
//  3. Returns the extracted Claims for use in subsequent interceptors or handlers
//
// Usage example:
//
//	provider := oidctools.NewProvider(...)
//	validator := oidc.NewDefaultValidator(provider)
//	authFunc := oidc.AuthFunc(validator)
//
//	// Use with gRPC server interceptor
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(auth.UnaryServerInterceptor(authFunc)),
//	)
//
// Returns an auth.Func that can be used with gRPC server interceptors.
// Authentication failures return gRPC Unauthenticated errors.
func AuthFunc(validator Validator) auth.AuthFunc {
	return func(ctx context.Context, request auth.Request) (any, error) {
		tokenCred, ok := request.TokenCredentials()
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "Missing token")
		}

		claims, err := validator.ValidateToken(ctx, tokenCred.Token)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "Token validation failed")
		}
		return claims, nil
	}
}
