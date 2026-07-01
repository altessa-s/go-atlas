// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package principal

import "slices"

// Principal is the canonical verified identity an authenticated request carries:
// the subject, its tenant, the granted scopes and roles, and an escape hatch of
// raw claims for anything not promoted to a field. Authentication adapters
// (oidc, mtls, selfjwt) build one and stash it as the request's principal so a
// service reads identity through one shape instead of reinventing its own.
//
// It is deliberately a plain data type with no decision logic: the generic
// decision cores ([github.com/altessa-s/go-atlas/auth/scope.Enforcer] and
// friends) stay parameterized over any P and read what they need through caller-
// supplied extractors. Principal is simply a convenient, standard P — its
// Scopes/Roles plug straight into scope.ScopeAuthorizer / scope.RoleAuthorizer
// (scope.Scope is a string alias), and its accessors cover the common checks.
type Principal struct {
	// Subject is the authenticated subject (the token sub, or a certificate
	// identity). Empty means anonymous.
	Subject string

	// Tenant scopes the principal to an organization/workspace. Empty when the
	// deployment is single-tenant or the claim is absent.
	Tenant string

	// Scopes are the granted authorization scopes (OAuth scope, RFC 9068).
	Scopes []string

	// Roles are the granted roles, mapped to scopes by a role registry where the
	// deployment is role-based.
	Roles []string

	// Claims carries raw claims not promoted to a field, for service-specific
	// needs. It may be nil.
	Claims map[string]any
}

// HasScope reports whether scope s was granted.
func (p Principal) HasScope(s string) bool { return slices.Contains(p.Scopes, s) }

// HasAnyScope reports whether at least one of scopes was granted. With no
// arguments it reports false.
func (p Principal) HasAnyScope(scopes ...string) bool {
	return slices.ContainsFunc(scopes, p.HasScope)
}

// HasAllScopes reports whether every one of scopes was granted. With no
// arguments it reports true (vacuously).
func (p Principal) HasAllScopes(scopes ...string) bool {
	for _, s := range scopes {
		if !p.HasScope(s) {
			return false
		}
	}
	return true
}

// HasRole reports whether role r was granted.
func (p Principal) HasRole(r string) bool { return slices.Contains(p.Roles, r) }

// HasAnyRole reports whether at least one of roles was granted. With no
// arguments it reports false.
func (p Principal) HasAnyRole(roles ...string) bool {
	return slices.ContainsFunc(roles, p.HasRole)
}

// Claim returns the raw claim named name. The bool is false when Claims is nil
// or the claim is absent.
func (p Principal) Claim(name string) (any, bool) {
	v, ok := p.Claims[name]
	return v, ok
}

// IsZero reports whether the principal carries no identity (anonymous): no
// subject, tenant, scopes, roles, or claims.
func (p Principal) IsZero() bool {
	return p.Subject == "" && p.Tenant == "" &&
		len(p.Scopes) == 0 && len(p.Roles) == 0 && len(p.Claims) == 0
}

// ClaimsSource is the narrow view of a verified claim set that [FromClaims]
// reads. It is satisfied by [github.com/altessa-s/go-atlas/auth/jwt.Claims] and
// any other claim type exposing the same accessors, so the mapping stays
// decoupled from a particular token package.
type ClaimsSource interface {
	// Subject returns the sub claim.
	Subject() string
	// Scopes returns the granted scopes (the scope claim).
	Scopes() []string
	// String returns the named claim as a string, ok=false when absent or not a
	// string.
	String(name string) (string, bool)
	// StringSlice returns the named claim as a string slice, ok=false when absent
	// or not string-valued.
	StringSlice(name string) ([]string, bool)
}

// FromClaims builds a Principal from a verified claim set, mapping sub→Subject
// and the scope claim→Scopes, plus the tenant and roles claims whose names are
// configurable ([WithTenantClaim] / [WithRolesClaim], defaulting to "tenant" and
// "roles"). The raw Claims map is left nil; set it on the result when a service
// needs claims beyond the promoted fields.
func FromClaims(c ClaimsSource, opts ...Option) Principal {
	o := newOptions(opts...)
	tenant, _ := c.String(o.tenantClaim)
	roles, _ := c.StringSlice(o.rolesClaim)
	return Principal{
		Subject: c.Subject(),
		Tenant:  tenant,
		Scopes:  c.Scopes(),
		Roles:   roles,
	}
}
