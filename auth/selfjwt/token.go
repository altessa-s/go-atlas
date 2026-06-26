// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token is the verified result of [Verifier.Verify]: the registered claims a
// caller needs plus the granted scopes. Any richer principal model (roles,
// superadmin flags) is the caller's concern and is built on top of this.
type Token struct {
	// Subject is the verified sub claim (the tenant or principal id).
	Subject string
	// ID is the jti claim (the token's unique id).
	ID string
	// Scopes are the granted permission scopes.
	Scopes []string
	// Expiry is the exp claim.
	Expiry time.Time
}

// tokenClaims is the JWT claim set for self-issued tokens: the registered
// claims plus the granted permission scopes. It satisfies jwt.Claims through
// the embedded jwt.RegisteredClaims.
type tokenClaims struct {
	jwt.RegisteredClaims
	Scopes []string `json:"scopes,omitempty"`
}
