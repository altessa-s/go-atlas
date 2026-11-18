// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/altessa-s/go-atlas-net/httpx"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpcmetadata "google.golang.org/grpc/metadata"
)

// ExtractBearerToken creates a TokenExtractor that extracts and validates Bearer tokens
// from incoming gRPC metadata. It requires exactly one Authorization value
// containing a case-insensitive Bearer scheme, one space, and a token68 credential.
// Missing or malformed credentials return codes.Unauthenticated.
func ExtractBearerToken() TokenExtractor {
	return TokenExtractorFunc(func(ctx context.Context) (string, error) {
		token, err := httpx.ParseBearer(grpcmetadata.ValueFromIncomingContext(ctx, "authorization"))
		if errors.Is(err, httpx.ErrBearerMissing) {
			return "", status.Error(codes.Unauthenticated, "missing or invalid token")
		}
		if err != nil {
			return "", status.Error(codes.Unauthenticated, "missing or invalid bearer token")
		}
		return token, nil
	})
}

// ExtractTokenFromHeader creates a TokenExtractor that extracts and validates tokens
// from a specified gRPC metadata header. This is a generic function that allows custom
// token extraction and validation logic for different authentication schemes.
//
// Parameters:
//   - header: The metadata header key to extract the token from (case-insensitive)
//   - fn: A function that receives the raw header value and returns the extracted token
//     and any validation error
//
// The extraction function receives the trimmed header value and should:
//  1. Validate the header format (e.g., check for required prefixes)
//  2. Extract and return the token value
//  3. Return an error if the format is invalid
//
// Returns a TokenExtractor that can be used with WithTokenExtractor option.
//
// Example usage for custom API key authentication:
//
//	extractor := auth.ExtractTokenFromHeader("x-api-key", func(value string) (string, error) {
//	    if value == "" {
//	        return "", status.Error(codes.Unauthenticated, "empty API key")
//	    }
//	    // Additional validation logic
//	    return value, nil
//	})
//
// Common use cases:
//   - Bearer tokens: Use ExtractBearerToken() for standard implementation
//   - API keys: Extract from custom headers (X-API-Key, X-Auth-Token, etc.)
//   - Basic auth: Parse and validate Basic authentication headers
//   - Custom schemes: Implement any authentication scheme with custom validation
//
// Security note: The extraction function should perform only format validation,
// not cryptographic validation. Token validation should be done in the auth function.
func ExtractTokenFromHeader(header string, fn func(t string) (string, error)) TokenExtractor {
	return TokenExtractorFunc(func(ctx context.Context) (token string, err error) {
		// Read the single key rather than copying all incoming metadata.
		if authHeaders := grpcmetadata.ValueFromIncomingContext(ctx, header); len(authHeaders) > 0 {
			authHeader := strings.TrimSpace(authHeaders[0])
			token, err = fn(authHeader)
		}

		if token == "" && err == nil {
			err = status.Error(codes.Unauthenticated, "missing or invalid token")
		}
		return
	})
}
