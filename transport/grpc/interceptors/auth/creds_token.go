// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import "github.com/altessa-s/go-atlas/core/types/redacted"

// TokenCredentials contains authentication fields specific to token-based authentication.
// This struct is used to pass token data from the token extractor to the auth function,
// and is the payload type for token-based authentication requests.
//
// The token value is extracted by a TokenExtractor (e.g., ExtractBearerToken) from the
// request metadata and wrapped in this struct before being passed to the auth function
// for validation.
//
// Example usage in an auth function:
//
//	authFunc := func(ctx context.Context, req auth.Request) (any, error) {
//	    tokenCreds, ok := req.TokenCredentials()
//	    if !ok {
//	        return nil, status.Error(codes.Unauthenticated, "invalid credentials")
//	    }
//
//	    // Validate the token (call Expose to read the raw value)
//	    userID, err := validateToken(tokenCreds.Token.Expose())
//	    if err != nil {
//	        return nil, status.Error(codes.Unauthenticated, "invalid token")
//	    }
//
//	    // Return custom data to be stored in Credentials.Data
//	    return userID, nil
//	}
//
// Security note: The Token field holds the raw token value as a
// [redacted.RedactedString], so it renders as a placeholder in logs, JSON,
// YAML, BSON, and slog output. Call Token.Expose() to read the raw value for
// validation, and always use timing-safe comparison functions.
type TokenCredentials struct {
	// Token is the actual token value extracted from the request.
	// This can be a Bearer token, API key, or any other string-based authentication token
	// depending on the TokenExtractor configuration.
	//
	// Common token formats:
	//   - JWT tokens: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
	//   - API keys: "sk_live_1234567890abcdef"
	//   - Opaque tokens: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	//
	// It is typed as [redacted.RedactedString] so accidental serialization
	// (structured logging, error reports) never leaks the raw token. Read the
	// underlying value with Token.Expose(). Always validate tokens using
	// cryptographic methods or secure comparison functions; never compare with
	// standard string equality (==) to prevent timing attacks.
	Token redacted.RedactedString
}
