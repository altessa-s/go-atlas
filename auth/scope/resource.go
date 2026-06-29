// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scope

// ResourceAuthorizer reports whether principal p may perform an action requiring
// the given scope on resource r. It is the object-level analog of [Authorizer]
// and the sole extension point of a [ResourceEnforcer]: scope matching, ownership
// and tenant checks, superuser bypass, and any principal- or resource-specific
// logic all live here, so the core stays agnostic of what a principal or a
// resource is.
//
// Reuse the action-level toolbox by lifting an [Authorizer] with [LiftAuthorizer]
// and combine it with an ownership predicate via [ResourceAllOf]:
//
//	scoped := scope.LiftAuthorizer[*Principal, *Doc](
//	    scope.ScopeAuthorizer(func(p *Principal) []scope.Scope { return p.Scopes }, scope.Exact()))
//	owns := func(p *Principal, d *Doc, _ scope.Scope) bool { return d.OwnerID == p.ID }
//	authorize := scope.ResourceAllOf(scoped, owns) // has the scope AND owns the resource
type ResourceAuthorizer[P, R any] func(p P, r R, required Scope) bool

// LiftAuthorizer adapts an action-level [Authorizer] into a [ResourceAuthorizer]
// that ignores the resource. Use it to reuse [ScopeAuthorizer], [AnyOf] / [AllOf]
// compositions, and superuser bypasses in object-level decisions. Both type
// parameters must be supplied explicitly, since R cannot be inferred from a.
func LiftAuthorizer[P, R any](a Authorizer[P]) ResourceAuthorizer[P, R] {
	return func(p P, _ R, required Scope) bool {
		return a(p, required)
	}
}

// ResourceAnyOf composes resource authorizers with logical OR: the result grants
// access as soon as one of them does (short-circuiting), and denies if none do.
// With no authorizers it always denies — the identity for OR. Use it for
// "owns the resource OR is a superuser" style rules.
func ResourceAnyOf[P, R any](authorizers ...ResourceAuthorizer[P, R]) ResourceAuthorizer[P, R] {
	return func(p P, r R, required Scope) bool {
		for _, a := range authorizers {
			if a(p, r, required) {
				return true
			}
		}
		return false
	}
}

// ResourceAllOf composes resource authorizers with logical AND: the result grants
// access only when every one of them does (short-circuiting on the first denial).
// With no authorizers it always grants — the identity for AND. Use it to layer
// the ownership check on top of the lifted scope check, e.g. "has the scope AND
// owns the resource".
func ResourceAllOf[P, R any](authorizers ...ResourceAuthorizer[P, R]) ResourceAuthorizer[P, R] {
	return func(p P, r R, required Scope) bool {
		for _, a := range authorizers {
			if !a(p, r, required) {
				return false
			}
		}
		return true
	}
}

// ResourceEnforcer decides whether principals of type P may perform registered
// actions on resources of type R. Like [Enforcer] it pairs a [Registry] (which
// action requires which scope, deny-by-default) with a [ResourceAuthorizer]
// (whether a principal may act on a resource). It holds no state beyond those two
// and is safe for concurrent use once the registry is frozen.
type ResourceEnforcer[P, R any] struct {
	reg       *Registry
	authorize ResourceAuthorizer[P, R]
}

// NewResourceEnforcer returns a [ResourceEnforcer] backed by reg, delegating the
// principal-may-act-on-resource decision to authorize.
func NewResourceEnforcer[P, R any](reg *Registry, authorize ResourceAuthorizer[P, R]) *ResourceEnforcer[P, R] {
	return &ResourceEnforcer[P, R]{reg: reg, authorize: authorize}
}

// Enforce reports whether principal p may perform the action identified by key on
// resource r. It returns nil when access is allowed and [ErrAccessDenied]
// otherwise. The policy is deny-by-default and fail-closed, matching
// [Enforcer.Enforce]:
//
//   - an unregistered key is denied — an action must be registered to be
//     reachable;
//   - a key registered with the empty scope is public and always allowed; the
//     resource is not consulted;
//   - otherwise the [ResourceAuthorizer] decides whether p may act on r.
func (e *ResourceEnforcer[P, R]) Enforce(p P, r R, key string) error {
	required, ok := e.reg.Required(key)
	if !ok {
		return ErrAccessDenied
	}
	if required == "" {
		return nil
	}
	if !e.authorize(p, r, required) {
		return ErrAccessDenied
	}
	return nil
}
