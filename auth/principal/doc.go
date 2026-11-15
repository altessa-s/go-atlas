// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package principal defines the canonical verified-identity type shared across
// the auth stack. A [Principal] carries the subject, tenant, granted scopes and
// roles, and an escape hatch of raw claims, so authentication adapters can stash
// one standard shape as a request's principal and services stop reinventing
// their own.
//
// # Relationship to the decision cores
//
// The generic authorization cores stay parameterized over any P and read what
// they need through caller-supplied extractors — Principal does not change that.
// It is simply a convenient, standard P. Because
// [github.com/altessa-s/go-atlas/auth/scope.Scope] is a string alias, a
// principal's Scopes and Roles plug straight in:
//
//	enf := scope.NewEnforcer(reg, scope.ScopeAuthorizer(
//	    func(p principal.Principal) []scope.Scope { return p.Scopes }, scope.Exact))
//	// role-based:
//	scope.RoleAuthorizer(func(p principal.Principal) []string { return p.Roles }, rs, scope.Exact)
//
// # Building from claims
//
// [FromClaims] maps a verified claim set onto a Principal through the narrow
// [ClaimsSource] interface, which [github.com/altessa-s/go-atlas/auth/jwt.Claims]
// satisfies:
//
//	claims, _ := verifier.Verify(ctx, raw)
//	p := principal.FromClaims(claims, principal.WithRolesClaim("groups"))
//
// # Request context
//
// [NewContext] stores a Principal in a context and [FromContext] reads it
// back. [NewContextFrom] stores a result held as an interface value without
// boxing it again. The HTTP auth middleware and the gRPC auth interceptor
// call it with their authenticator's result, so whenever that is a Principal
// (or a non-nil *Principal) a handler served over both transports reads the
// caller uniformly:
//
//	if p, ok := principal.FromContext(ctx); ok && p.HasScope("files:read") {
//	    // ...
//	}
//
// Authenticators that return an application-defined identity type are not
// bridged; their result stays reachable through the transport's own context
// accessor.
package principal
