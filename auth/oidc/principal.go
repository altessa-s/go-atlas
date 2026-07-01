// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"github.com/altessa-s/go-atlas/auth/principal"

	authjwt "github.com/altessa-s/go-atlas/auth/jwt"
)

// Principal maps a verified OIDC claim set onto the canonical
// [principal.Principal], for callers standardizing on that type as their
// authorization subject across transports. The claims are the validated map a
// [Provider] returns; sub→Subject, the scope claim→Scopes, and the configurable
// tenant/roles claims are promoted ([principal.WithTenantClaim] /
// [principal.WithRolesClaim]). It is a thin convenience over
// [principal.FromClaims] — the provider's verification and revocation pipeline
// is unchanged.
func Principal(claims map[string]any, opts ...principal.Option) principal.Principal {
	return principal.FromClaims(authjwt.Claims(claims), opts...)
}
