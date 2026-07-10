// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scope

// Authorizer reports whether principal p is granted the required scope. It is
// the sole extension point of an [Enforcer]: scope matching, superuser bypass,
// role or tenant checks, and any principal-specific fields all live here, so the
// core stays agnostic of what a principal is. Build the common case with
// [ScopeAuthorizer] and compose extra rules around it:
//
//	base := scope.ScopeAuthorizer(func(p *Principal) []string { return p.Scopes }, scope.Exact())
//	authorize := func(p *Principal, required scope.Scope) bool {
//	    return p.Superuser || base(p, required) // superuser is the caller's concept, not the core's
//	}
type Authorizer[P any] func(p P, required Scope) bool

// ScopeAuthorizer builds the common [Authorizer]: principal p is granted
// required when its scopes (returned by scopesOf) satisfy required under m.
func ScopeAuthorizer[P any](scopesOf func(P) []Scope, m Matcher) Authorizer[P] {
	return func(p P, required Scope) bool {
		return m(scopesOf(p), required)
	}
}

// AnyOf composes authorizers with logical OR: the result grants access as soon
// as one of them does (short-circuiting), and denies if none do. With no
// authorizers it always denies — the identity for OR. Use it for "satisfies the
// scope OR is a superuser" style rules:
//
//	authorize := scope.AnyOf(
//	    scope.ScopeAuthorizer(scopesOf, scope.Exact()),
//	    func(p *Principal, _ scope.Scope) bool { return p.Superuser },
//	)
func AnyOf[P any](authorizers ...Authorizer[P]) Authorizer[P] {
	return func(p P, required Scope) bool {
		for _, a := range authorizers {
			if a(p, required) {
				return true
			}
		}
		return false
	}
}

// AllOf composes authorizers with logical AND: the result grants access only
// when every one of them does (short-circuiting on the first denial). With no
// authorizers it denies — fail-closed, so a mis-wired empty AllOf (e.g.
// NewEnforcer(reg, scope.AllOf())) can never silently grant every action. Use it
// to layer an extra gate on top of the scope check, e.g. "has the scope AND the
// tenant is active".
func AllOf[P any](authorizers ...Authorizer[P]) Authorizer[P] {
	return func(p P, required Scope) bool {
		for _, a := range authorizers {
			if !a(p, required) {
				return false
			}
		}
		return len(authorizers) > 0
	}
}

// Enforcer decides whether principals of type P may perform registered actions.
// It pairs a [Registry] (which action requires which scope, deny-by-default)
// with an [Authorizer] (whether a principal satisfies a scope). It holds no
// state beyond those two and is safe for concurrent use once the registry is
// frozen.
type Enforcer[P any] struct {
	reg       *Registry
	authorize Authorizer[P]
}

// NewEnforcer returns an [Enforcer] backed by reg, delegating the
// principal-satisfies-scope decision to authorize.
func NewEnforcer[P any](reg *Registry, authorize Authorizer[P]) *Enforcer[P] {
	return &Enforcer[P]{reg: reg, authorize: authorize}
}

// Enforce reports whether principal p may perform the action identified by key.
// It returns nil when access is allowed and [ErrAccessDenied] otherwise. The
// policy is deny-by-default and fail-closed:
//
//   - an unregistered key is denied — an action must be registered to be
//     reachable;
//   - a key registered with the empty scope is public and always allowed;
//   - otherwise the [Authorizer] decides whether p satisfies the required scope.
func (e *Enforcer[P]) Enforce(p P, key string) error {
	required, ok := e.reg.Required(key)
	if !ok {
		return ErrAccessDenied
	}
	if required == "" {
		return nil
	}
	if !e.authorize(p, required) {
		return ErrAccessDenied
	}
	return nil
}
