// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package oidc provides OpenID Connect JWT token validation with automatic JWKS key rotation.
// It supports signature verification, claims validation, token introspection (RFC 7662),
// validation presets with matchers (native and CEL-based), and userinfo retrieval.
//
// Example:
//
//	provider, _ := oidc.NewProvider(ctx,
//	    "https://accounts.google.com/.well-known/openid-configuration",
//	    oidc.WithDefaultValidationOptions(
//	        oidc.WithValidationIssuer("https://accounts.google.com"),
//	        oidc.WithValidationAudience("my-client-id"),
//	    ),
//	)
//	defer provider.Close()
//	claims, _ := provider.ValidateToken(ctx, token)
package oidc
