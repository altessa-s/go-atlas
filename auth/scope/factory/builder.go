// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"

	"github.com/altessa-s/go-atlas/auth/scope"
	"github.com/altessa-s/go-atlas/config"
)

// RegistryBuilder assembles a frozen [scope.Registry] from a
// [config.ScopeRegistry]. It is the config-driven counterpart to building a
// registry by hand with [scope.NewRegistry] + [scope.Registry.Register], giving
// the scope subsystem the same config→component path the OPA factory provides.
type RegistryBuilder struct {
	cfg *config.ScopeRegistry
}

// New creates a [RegistryBuilder] for the given configuration. A nil cfg is
// accepted; the error surfaces at [RegistryBuilder.Build] time.
func New(cfg *config.ScopeRegistry) *RegistryBuilder {
	return &RegistryBuilder{cfg: cfg}
}

// Build registers every configured rule and returns the frozen [scope.Registry],
// ready for lock-free concurrent reads. The registry only models which action
// requires which scope; pair it with an [scope.Authorizer] via
// [scope.NewEnforcer] in code, since the authorizer depends on the caller's
// principal type.
//
// Build fails when the configuration is nil, when a rule lists no keys, or when
// an action key appears in more than one rule — an ambiguous policy is a
// configuration error, not a last-write-wins surprise.
func (b *RegistryBuilder) Build() (*scope.Registry, error) {
	if b.cfg == nil {
		return nil, fmt.Errorf("scope/factory: configuration is required")
	}

	reg := scope.NewRegistry()
	seen := make(map[string]struct{})
	for i, rule := range b.cfg.Rules {
		if len(rule.Keys) == 0 {
			return nil, fmt.Errorf("scope/factory: rule %d has no keys", i)
		}
		for _, key := range rule.Keys {
			if _, dup := seen[key]; dup {
				return nil, fmt.Errorf("scope/factory: duplicate action key %q", key)
			}
			seen[key] = struct{}{}
		}
		reg.RegisterMany(rule.Scope, rule.Keys...)
	}
	reg.Freeze()
	return reg, nil
}
