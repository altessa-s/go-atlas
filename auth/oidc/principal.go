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

// Principal maps c onto the canonical [principal.Principal]: Subject and
// Scopes from the typed fields, Tenant and Roles from the default "tenant" and
// "roles" claims in RawClaims. The HTTP and gRPC OIDC auth adapters return
// *Claims, and the transport auth layers call this method to expose the caller
// through principal.FromContext as well. For other tenant or roles claim names,
// map RawClaims with the package-level [Principal] in a custom authenticator.
// A nil c maps to the zero Principal.
func (c *Claims) Principal() principal.Principal {
	if c == nil {
		return principal.Principal{}
	}
	return principal.FromClaims(claimsView{c})
}

// claimsView adapts *Claims to [principal.ClaimsSource]: the typed fields for
// the subject and scopes, RawClaims for everything else.
type claimsView struct{ c *Claims }

func (v claimsView) Subject() string  { return v.c.Subject }
func (v claimsView) Scopes() []string { return v.c.Scopes }

func (v claimsView) String(name string) (string, bool) {
	return authjwt.Claims(v.c.RawClaims).String(name)
}

func (v claimsView) StringSlice(name string) ([]string, bool) {
	return authjwt.Claims(v.c.RawClaims).StringSlice(name)
}
