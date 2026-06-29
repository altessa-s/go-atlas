// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validator

import (
	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc"
)

// extractClaims projects a verified raw claim set onto the typed [oidc.Claims].
//
// Every field is read through the shared github.com/altessa-s/go-atlas/auth/jwt
// [jwt.Claims] accessors, so OIDC claim parsing — string coercion, audience
// normalization (single string or array), the space-separated-or-array scope
// claim, and numeric-date decoding — has one canonical implementation rather
// than a second hand-rolled copy. The raw map is retained verbatim in RawClaims
// for access to custom claims not represented as fields.
func extractClaims(raw map[string]any) *oidc.Claims {
	jc := jwt.Claims(raw)
	preferredUsername, _ := jc.String("preferred_username")
	email, _ := jc.String("email")
	familyName, _ := jc.String("family_name")
	name, _ := jc.String("name")
	givenName, _ := jc.String("given_name")
	return &oidc.Claims{
		Subject:           jc.Subject(),
		PreferredUsername: preferredUsername,
		Email:             email,
		Issuer:            jc.Issuer(),
		Audience:          jc.Audience(),
		Scopes:            jc.Scopes(),
		ExpiresAt:         jc.Expiry(),
		IssuedAt:          jc.IssuedAt(),
		NotBefore:         jc.NotBefore(),
		FamilyName:        familyName,
		Name:              name,
		GivenName:         givenName,
		RawClaims:         raw,
	}
}
