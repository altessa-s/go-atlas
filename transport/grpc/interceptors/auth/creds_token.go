// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

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
//	    // Validate the token
//	    userID, err := validateToken(tokenCreds.Token)
//	    if err != nil {
//	        return nil, status.Error(codes.Unauthenticated, "invalid token")
//	    }
//
//	    // Return custom data to be stored in Credentials.Data
//	    return userID, nil
//	}
//
// Security note: The Token field contains the raw token value as extracted from the
// request. This should be handled securely and not logged in plaintext. Always use
// timing-safe comparison functions when validating tokens.
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
	// Security: Always validate tokens using cryptographic methods or secure comparison
	// functions. Never compare tokens using standard string equality (==) to prevent
	// timing attacks.
	Token string
}
