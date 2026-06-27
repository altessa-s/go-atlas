// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package oidc provides OIDC (OpenID Connect) token validation support for the auth interceptor.
// It integrates with OIDC providers to validate JWT tokens and extract standard OIDC claims.
//
// # Overview
//
// This package bridges the gap between the auth interceptor and OIDC providers, providing:
//   - Token validation against OIDC providers
//   - Structured claim extraction from JWT tokens
//   - Integration with the auth.AuthFunc interface
//
// # Basic Usage
//
// Create an auth function that validates OIDC tokens:
//
//	import (
//	    "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
//	    "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc"
//	    "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc/validator"
//	    "git.altessa-s.com/altessa/go-tools/v2/oidc/providers/keycloak"
//	)
//
//	// Setup OIDC provider
//	provider, err := keycloak.NewProvider(
//	    keycloak.WithIssuerURL("https://auth.example.com/realms/myrealm"),
//	    keycloak.WithClientID("my-service"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Create validator
//	oidcValidator := validator.NewDefaultValidator(provider)
//
//	// Create auth function
//	authFunc := oidc.AuthFunc(oidcValidator)
//
//	// Use with gRPC server
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(auth.ServerUnaryInterceptor(
//	        auth.WithAuthFunc(authFunc),
//	    )),
//	)
//
// # Accessing Claims in Handlers
//
// After successful authentication, OIDC claims are available in the context:
//
//	func (s *MyService) GetUser(ctx context.Context, req *GetUserRequest) (*User, error) {
//	    creds, ok := auth.CredentialsFromContext(ctx)
//	    if !ok {
//	        return nil, status.Error(codes.Unauthenticated, "no credentials")
//	    }
//
//	    // Claims are stored in the Data field
//	    claims := creds.Data.(map[string]any)
//	    subject := claims["sub"].(string)
//	    email := claims["email"].(string)
//
//	    return s.getUserBySubject(subject)
//	}
//
// # Custom Validators
//
// Implement the Validator interface for custom validation logic:
//
//	type CustomValidator struct {
//	    provider oidc.Provider
//	}
//
//	func (v *CustomValidator) ValidateToken(ctx context.Context, token string) (map[string]any, error) {
//	    // Validate token using provider
//	    claims, err := v.provider.ValidateToken(ctx, token)
//	    if err != nil {
//	        return nil, err
//	    }
//
//	    // Additional validation logic
//	    if claims["tenant_id"] != "expected-tenant" {
//	        return nil, errors.New("invalid tenant")
//	    }
//
//	    return claims, nil
//	}
//
// # Integration with scope-based authorization
//
// Keep the AuthFunc focused on authentication — return the verified claims — and
// layer authorization on with the transport-neutral
// github.com/altessa-s/go-atlas/auth/scope package, wired through the auth
// interceptor's ScopeClientAuth. Deny-by-default and the method→scope policy
// live in the scope.Enforcer, not in the AuthFunc:
//
//	authFunc := auth.AuthFunc(func(ctx context.Context, req auth.Request) (any, error) {
//	    tokenCreds, ok := req.TokenCredentials()
//	    if !ok {
//	        return nil, status.Error(codes.Unauthenticated, "invalid credentials")
//	    }
//	    claims, err := oidcValidator.ValidateToken(ctx, tokenCreds.Token.Expose())
//	    if err != nil {
//	        return nil, status.Error(codes.Unauthenticated, "token validation failed")
//	    }
//	    return claims, nil // *oidc.Claims becomes Credentials.Data
//	})
//
//	reg := scope.NewRegistry()
//	reg.RegisterMany("user:read", "/user.UserService/GetUser")
//	reg.Freeze()
//	enf := scope.NewEnforcer(reg, scope.ScopeAuthorizer(
//	    func(c *oidc.Claims) []string { return c.Scopes }, scope.Exact()))
//
//	interceptor := auth.ServerInterceptor(
//	    auth.WithAuthFunc(authFunc),
//	    auth.WithClientAuth(auth.ScopeClientAuth(enf)),
//	)
//
// # Claims Structure
//
// The Claims struct provides typed access to standard OIDC claims:
//   - Subject (sub): Unique user identifier
//   - Email: User's email address
//   - PreferredUsername: User's preferred username
//   - Scopes: OAuth 2.0 scopes
//   - Issuer (iss): Token issuer URL
//   - Audience (aud): Intended recipients
//   - ExpiresAt (exp): Token expiration time
//   - IssuedAt (iat): Token issuance time
//   - NotBefore (nbf): Token validity start time
//   - Name: Full name
//   - GivenName: First name
//   - FamilyName: Last name
//   - RawClaims: All claims as map[string]any
//
// # Validator Package
//
// The validator subpackage provides a default OIDC token validator implementation
// that extracts standard claims and handles common OIDC validation scenarios.
// See the validator package documentation for more details.
//
// # Security Considerations
//
//   - Always verify token signatures using the OIDC provider
//   - Validate token expiration (exp) and not-before (nbf) times
//   - Verify the issuer (iss) matches your expected OIDC provider
//   - Check the audience (aud) includes your service
//   - Use HTTPS for all OIDC provider communication
//   - Implement proper error handling to avoid information leakage
//   - Consider implementing token caching with TTL
//
// # Performance
//
//   - Token validation involves cryptographic operations (JWT signature verification)
//   - JWKS keys are typically cached by the OIDC provider implementation
//   - Consider implementing application-level token caching for high-traffic services
//   - Claims extraction is lightweight (JSON parsing)
package oidc
