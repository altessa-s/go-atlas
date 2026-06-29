// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory builds a frozen [github.com/altessa-s/go-atlas/auth/scope.Registry]
// from a [github.com/altessa-s/go-atlas/config.ScopeRegistry]. It is the
// config-driven counterpart to constructing a registry by hand, giving the scope
// subsystem the same config→component path the OPA factory provides.
//
// Only the action-key→required-scope table is declarative. The matcher and the
// authorizer stay in code: they depend on the caller's principal type, which has
// no place in configuration.
//
// # Usage
//
//	reg, err := factory.New(&cfg.Scope).Build() // cfg.Scope is a config.ScopeRegistry
//	if err != nil {
//	    return err
//	}
//	enf := scope.NewEnforcer(reg, scope.ScopeAuthorizer(scopesOf, scope.Exact()))
package factory
