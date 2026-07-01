// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"time"

	"github.com/altessa-s/go-atlas/auth/principal"
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

// Principal returns the verified identity as a canonical
// [principal.Principal] — its Subject and Scopes — for callers standardizing on
// that type as their authorization subject across transports. A self-issued
// token carries no tenant, roles, or extra claims, so those stay empty.
func (t Token) Principal() principal.Principal {
	return principal.Principal{Subject: t.Subject, Scopes: t.Scopes}
}
