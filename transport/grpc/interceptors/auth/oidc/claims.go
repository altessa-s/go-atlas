// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import "time"

// Claims represents the structured information extracted from a validated OIDC token.
// It provides typed access to standard OpenID Connect Core 1.0 claims, making it
// easier to work with OIDC tokens without manual claim extraction and type assertions.
//
// The Claims struct follows the OpenID Connect Core 1.0 specification for standard
// claims and includes both required and optional claims commonly used in OIDC flows.
//
// Example usage:
//
//	claims, err := validator.ValidateToken(ctx, token)
//	if err != nil {
//	    return err
//	}
//
//	// Access typed claims
//	log.Printf("User: %s (%s)", claims.PreferredUsername, claims.Email)
//	log.Printf("Scopes: %v", claims.Scopes)
//	log.Printf("Expires: %s", claims.ExpiresAt)
//
//	// Access custom claims from RawClaims
//	tenantID := claims.RawClaims["tenant_id"].(string)
//
// For additional or non-standard claims not represented in the struct fields,
// use the RawClaims map which contains all claims from the original token.
type Claims struct {
	// Subject is the unique identifier for the authenticated user within the issuer.
	// This is the primary key for identifying users and is guaranteed to be unique
	// within the context of the OIDC provider. Corresponds to the "sub" claim.
	//
	// Example: "248289761001" or "user-abc-123"
	Subject string `json:"sub"`

	// PreferredUsername is the user's preferred username for display purposes.
	// This may not be unique and can change over time. Use Subject for unique
	// identification. Corresponds to the "preferred_username" claim.
	//
	// Example: "john.doe" or "johndoe123"
	PreferredUsername string `json:"preferred_username"`

	// Email is the user's email address. The OIDC provider may or may not have
	// verified this email. Check the "email_verified" claim in RawClaims if
	// email verification status is important. Corresponds to the "email" claim.
	//
	// Example: "john.doe@example.com"
	Email string `json:"email"`

	// Issuer identifies the OIDC provider that issued the token. This should be
	// validated to ensure tokens come from trusted providers. Corresponds to
	// the "iss" claim.
	//
	// Example: "https://accounts.google.com" or "https://auth.example.com/realms/myrealm"
	Issuer string `json:"iss"`

	// Audience identifies the intended recipients of the token. Your service should
	// verify it's listed in the audience to prevent token misuse. Corresponds to
	// the "aud" claim which can be a string or array of strings.
	//
	// Example: ["my-service", "my-api"]
	Audience []string `json:"aud"`

	// Scopes lists the OAuth 2.0 scopes granted to the token. Use these for
	// authorization decisions. Scopes are extracted from the "scopes" claim
	// and automatically sorted alphabetically.
	//
	// Example: ["openid", "profile", "email", "user:read", "user:write"]
	Scopes []string `json:"scope"`

	// ExpiresAt is the time after which the token is no longer valid. Always
	// check token expiration before processing requests. Corresponds to the
	// "exp" claim (Unix timestamp).
	//
	// Example: 2025-01-15 10:30:00 UTC
	ExpiresAt time.Time `json:"exp"`

	// IssuedAt is the time when the token was issued by the OIDC provider.
	// Useful for detecting token age and implementing additional security
	// policies. Corresponds to the "iat" claim (Unix timestamp).
	//
	// Example: 2025-01-15 09:30:00 UTC
	IssuedAt time.Time `json:"iat"`

	// NotBefore is the time before which the token must not be accepted.
	// Tokens should only be considered valid after this time. Corresponds
	// to the "nbf" claim (Unix timestamp).
	//
	// Example: 2025-01-15 09:30:00 UTC
	NotBefore time.Time `json:"nbf"`

	// FamilyName is the user's last name or surname. This is an optional claim
	// and may be empty if not provided by the OIDC provider or if the user has
	// not shared this information. Corresponds to the "family_name" claim.
	//
	// Example: "Doe"
	FamilyName string `json:"family_name"`

	// Name is the user's full name in displayable form. This typically includes
	// all name components in the order preferred by the user's locale. Corresponds
	// to the "name" claim.
	//
	// Example: "John Doe" or "Doe, John"
	Name string `json:"name"`

	// GivenName is the user's first name. This is an optional claim and may be
	// empty if not provided by the OIDC provider or if the user has not shared
	// this information. Corresponds to the "given_name" claim.
	//
	// Example: "John"
	GivenName string `json:"given_name"`

	// RawClaims contains all claims from the original OIDC token, including both
	// standard claims (represented as struct fields above) and any custom claims
	// specific to your OIDC provider or application.
	//
	// Use this for:
	//   - Accessing custom/non-standard claims (e.g., tenant_id, roles, groups)
	//   - Checking optional claims not represented as struct fields (e.g., email_verified)
	//   - Preserving the complete token information for auditing or logging
	//
	// Example:
	//   tenantID := claims.RawClaims["tenant_id"].(string)
	//   emailVerified := claims.RawClaims["email_verified"].(bool)
	//   roles := claims.RawClaims["roles"].([]string)
	RawClaims map[string]any `json:"-"`
}

// ScopesOf returns the OAuth scopes granted to c, ready to use as the scopesOf
// argument of github.com/altessa-s/go-atlas/auth/scope.ScopeAuthorizer. A nil c
// yields no scopes:
//
//	enf := scope.NewEnforcer(reg, scope.ScopeAuthorizer(oidc.ScopesOf, scope.Exact()))
func ScopesOf(c *Claims) []string {
	if c == nil {
		return nil
	}
	return c.Scopes
}
