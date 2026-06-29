// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package validator provides OIDC token validation and claims extraction functionality.
// It implements a default validator that integrates with OIDC providers to verify tokens
// and extract standard OpenID Connect claims.
//
// # Overview
//
// This package provides:
//   - DefaultValidator: A production-ready OIDC token validator
//   - Claims extraction from JWT tokens with typed fields
//   - Support for standard OIDC claim formats
//   - Configurable logging for debugging
//
// # Basic Usage
//
// Create a validator and use it to validate OIDC tokens:
//
//	import (
//	    "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc/validator"
//	    "git.altessa-s.com/altessa/go-tools/v2/oidc/providers/keycloak"
//	)
//
//	// Create OIDC provider
//	provider, err := keycloak.NewProvider(
//	    keycloak.WithIssuerURL("https://auth.example.com/realms/myrealm"),
//	    keycloak.WithClientID("my-service"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Create validator with optional logging
//	v := validator.NewDefaultValidator(provider,
//	    validator.WithLogger(slog.Default()),
//	)
//
//	// Validate a token
//	claims, err := v.ValidateToken(ctx, tokenString)
//	if err != nil {
//	    log.Printf("Token validation failed: %v", err)
//	    return
//	}
//
//	// Access claims
//	log.Printf("User: %s", claims.Subject)
//	log.Printf("Email: %s", claims.Email)
//	log.Printf("Scopes: %v", claims.Scopes)
//
// # Claims Extraction
//
// The validator extracts the following standard OIDC claims:
//
// Identity claims:
//   - Subject (sub): Unique user identifier
//   - PreferredUsername (preferred_username): Display username
//   - Email (email): User's email address
//   - Name (name): Full display name
//   - GivenName (given_name): First name
//   - FamilyName (family_name): Last name
//
// Token metadata:
//   - Issuer (iss): Token issuer URL
//   - Audience (aud): Intended recipients (string or array)
//   - ExpiresAt (exp): Expiration timestamp
//   - IssuedAt (iat): Issuance timestamp
//   - NotBefore (nbf): Validity start timestamp
//
// Authorization:
//   - Scopes (scopes): OAuth 2.0 scopes (string, array, or space-separated)
//
// All claims are also available in the RawClaims map for custom claim access.
//
// # Scope Extraction
//
// The validator handles scopes in multiple formats:
//
//	// Space-separated string
//	"scope": "openid profile email user:read"
//
//	// Array of strings
//	"scope": ["openid", "profile", "email", "user:read"]
//
//	// Mixed array (non-string values filtered out)
//	"scope": ["openid", 123, "profile", null, "email"]
//
// Scopes are automatically:
//   - Filtered (empty strings removed)
//   - Preserved in original token order
//   - Returned as nil if no valid scopes found
//
// # Audience Handling
//
// The audience claim can be a single string or array of strings:
//
//	// Single audience
//	"aud": "my-service"
//	// Result: []string{"my-service"}
//
//	// Multiple audiences
//	"aud": ["my-service", "my-api", "admin-panel"]
//	// Result: []string{"my-service", "my-api", "admin-panel"}
//
// # Integration with Auth Interceptor
//
// Use with the OIDC auth function:
//
//	import (
//	    "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
//	    "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc"
//	    "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc/validator"
//	)
//
//	v := validator.NewDefaultValidator(provider)
//	authFunc := oidc.AuthFunc(v)
//
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(auth.ServerUnaryInterceptor(
//	        auth.WithAuthFunc(authFunc),
//	    )),
//	)
//
// # Custom Validation
//
// Implement the Provider interface for custom validation logic:
//
//	type CustomProvider struct {
//	    baseProvider oidc.Provider
//	}
//
//	func (p *CustomProvider) ValidateToken(ctx context.Context, token string) (map[string]any, error) {
//	    // Delegate to base provider for signature verification
//	    claims, err := p.baseProvider.ValidateToken(ctx, token)
//	    if err != nil {
//	        return nil, err
//	    }
//
//	    // Add custom validation
//	    if claims["tenant_id"] != expectedTenant {
//	        return nil, errors.New("invalid tenant")
//	    }
//
//	    return claims, nil
//	}
//
// # Error Handling
//
// Validation errors are returned as gRPC Unauthenticated errors:
//
//	claims, err := validator.ValidateToken(ctx, token)
//	if err != nil {
//	    // Error is already a gRPC status error (codes.Unauthenticated)
//	    return nil, err
//	}
//
// Common validation failures:
//   - Invalid signature: Token signature verification failed
//   - Expired token: Token exp claim is in the past
//   - Invalid issuer: Token iss doesn't match expected issuer
//   - Invalid audience: Token aud doesn't include this service
//   - Malformed token: Token cannot be parsed as JWT
//
// # Logging
//
// Configure logging for debugging and monitoring:
//
//	v := validator.NewDefaultValidator(provider,
//	    validator.WithLogger(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
//	        Level: slog.LevelDebug,
//	    }))),
//	)
//
// Logged events:
//   - Token validation failures (Error level)
//   - Claims extraction details (Debug level)
//
// # Security Considerations
//
//   - Token validation includes cryptographic signature verification
//   - Always verify exp, iat, and nbf timestamps
//   - Validate issuer matches your expected OIDC provider
//   - Check audience includes your service identifier
//   - Use HTTPS for all OIDC provider communication
//   - Don't log tokens in plaintext (security risk)
//   - Implement rate limiting on authentication endpoints
//   - Consider token caching with appropriate TTL
//
// # Performance
//
//   - JWT signature verification is the main performance cost
//   - JWKS keys are cached by the OIDC provider implementation
//   - Claims extraction is lightweight (JSON parsing)
//   - Single-pass extraction with minimal allocations
//   - Claim parsing is delegated to the shared auth/jwt accessors
//
// # Testing
//
// Mock the Provider interface for testing:
//
//	type MockProvider struct{}
//
//	func (m *MockProvider) ValidateToken(ctx context.Context, token string) (map[string]any, error) {
//	    if token == "valid-token" {
//	        return map[string]any{
//	            "sub": "user-123",
//	            "email": "user@example.com",
//	            "scope": []string{"openid", "profile"},
//	        }, nil
//	    }
//	    return nil, errors.New("invalid token")
//	}
//
//	validator := validator.NewDefaultValidator(&MockProvider{})
package validator
