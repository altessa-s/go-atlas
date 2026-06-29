// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package scope is a transport-neutral, deny-by-default authorization policy
// built on permission scopes. It answers one question — may this principal
// perform this action? — without any knowledge of transports, I/O, or what a
// principal is, so the same policy drives gRPC and HTTP authorization and is
// trivially unit-testable.
//
// # Model
//
// A [Registry] maps an action key (a gRPC full method, an HTTP route, any stable
// identifier) to the [Scope] required to perform it. It is built once, frozen,
// then read concurrently. Lookups are deny-by-default: an unregistered key is
// denied; a key registered with the empty scope is intentionally public.
//
// An [Enforcer] pairs that registry with an [Authorizer] — the single extension
// point. The Authorizer decides whether a principal of the caller's own type P
// satisfies a required scope. Scope matching, superuser bypass, role or tenant
// checks, and any extra principal fields live there, composed by the caller; the
// core bakes in no identity fields. Build the common case with [ScopeAuthorizer]
// over a [Matcher] ([Exact] or [Wildcard]).
//
// # Decision
//
// [Enforcer.Enforce] is fail-closed:
//
//   - unregistered key                  → [ErrAccessDenied]
//   - key registered with empty scope   → allowed (public)
//   - otherwise                         → the Authorizer decides
//
// [ErrAccessDenied] is transport-neutral; adapters map it (gRPC
// codes.PermissionDenied, HTTP 403).
//
// # Usage
//
//	type Principal struct {
//	    Scopes    []string
//	    Superuser bool
//	}
//
//	reg := scope.NewRegistry()
//	reg.Register("/files.v1.Files/Read", "files:read")
//	reg.Register("/files.v1.Files/Write", "files:write")
//	reg.Register("/health.v1.Health/Check", "") // public
//	reg.Freeze()
//
//	base := scope.ScopeAuthorizer(func(p *Principal) []string { return p.Scopes }, scope.Exact())
//	authorize := func(p *Principal, required scope.Scope) bool {
//	    return p.Superuser || base(p, required)
//	}
//	enf := scope.NewEnforcer(reg, authorize)
//
//	if err := enf.Enforce(p, "/files.v1.Files/Write"); err != nil {
//	    // errors.Is(err, scope.ErrAccessDenied)
//	}
//
// # Object-level authorization
//
// When the decision depends on the specific resource being acted on (ownership,
// per-object ACLs), use the [ResourceEnforcer] / [ResourceAuthorizer] variants.
// They mirror the action-level API with one extra resource parameter and the same
// deny-by-default registry and fail-closed [ResourceEnforcer.Enforce] semantics.
// Lift the scope and superuser logic with [LiftAuthorizer] and AND it with an
// ownership predicate via [ResourceAllOf]:
//
//	scoped := scope.LiftAuthorizer[*Principal, *Doc](base)
//	owns := func(p *Principal, d *Doc, _ scope.Scope) bool { return d.OwnerID == p.ID }
//	enf := scope.NewResourceEnforcer(reg, scope.ResourceAllOf(scoped, owns))
//
//	if err := enf.Enforce(p, doc, "/docs.v1.Docs/Update"); err != nil {
//	    // errors.Is(err, scope.ErrAccessDenied)
//	}
package scope
