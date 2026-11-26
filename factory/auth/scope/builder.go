// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"

	"github.com/altessa-s/go-atlas/auth/scope"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// RegistryBuilder assembles a frozen [scope.Registry] from a
// [authconfig.ScopeRegistry]. It is the config-driven counterpart to building a
// registry by hand with [scope.NewRegistry] + [scope.Registry.Register], giving
// the scope subsystem the same config→component path the OPA factory provides.
type RegistryBuilder struct {
	cfg *authconfig.ScopeRegistry
}

// New creates a [RegistryBuilder] for the given configuration. A nil cfg is
// accepted; the error surfaces at [RegistryBuilder.Build] time.
func New(cfg *authconfig.ScopeRegistry) *RegistryBuilder {
	return &RegistryBuilder{cfg: cfg}
}

// Build registers every configured rule and returns the frozen [scope.Registry],
// ready for lock-free concurrent reads. The registry only models which action
// requires which scope; pair it with an [scope.Authorizer] via
// [scope.NewEnforcer] in code, since the authorizer depends on the caller's
// principal type.
//
// Build rejects nil configuration, invalid rules, and duplicate action keys.
func (b *RegistryBuilder) Build() (*scope.Registry, error) {
	if b.cfg == nil {
		return nil, fmt.Errorf("scope/factory: %w", corefactory.ErrConfigRequired)
	}

	reg := scope.NewRegistry()
	seen := make(map[string]struct{})
	for i, rule := range b.cfg.Rules {
		if err := rule.Validate(); err != nil {
			return nil, fmt.Errorf("scope/factory: invalid rule %d: %w", i, err)
		}
		for _, key := range rule.Keys {
			if _, dup := seen[key]; dup {
				return nil, fmt.Errorf("scope/factory: duplicate action key in rule %d", i)
			}
			seen[key] = struct{}{}
		}
		if rule.Public {
			reg.RegisterPublic(rule.Keys...)
		} else {
			reg.RegisterMany(rule.Scope, rule.Keys...)
		}
	}
	reg.Freeze()
	return reg, nil
}
