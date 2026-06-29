// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// ScopeRule maps a set of action keys to the scope required to perform them. It
// mirrors a bulk [auth/scope.Registry.RegisterMany] call: every key in Keys is
// registered as requiring Scope.
type ScopeRule struct {
	// Scope is the scope required for the listed keys. An empty scope marks the
	// keys as intentionally public (always allowed once authenticated).
	Scope string `yaml:"scope"`

	// Keys are the action identifiers requiring Scope — gRPC full methods
	// ("/pkg.Service/Method"), HTTP route keys, or any stable action id.
	Keys []string `yaml:"keys"`
}

// Validate ensures a rule lists at least one key. An empty Scope is valid and
// denotes a public rule.
func (r ScopeRule) Validate() error {
	return ValidateStruct(&r,
		validation.Field(&r.Keys, validation.Required),
	)
}

// ScopeRegistry is the configuration for a scope authorization registry: the
// declarative action-key→required-scope table consumed by
// [auth/scope/factory.New]. It carries no principal, matcher, or authorizer
// concern — those stay in code, since they depend on the caller's principal type.
//
// Example:
//
//	scope:
//	  rules:
//	    - scope: "files:read"
//	      keys: ["/files.v1.Files/Read", "/files.v1.Files/List"]
//	    - scope: "files:write"
//	      keys: ["/files.v1.Files/Write"]
//	    - scope: ""               # public
//	      keys: ["/health.v1.Health/Check"]
type ScopeRegistry struct {
	// Rules is the ordered list of key→scope rules. Each action key must appear
	// at most once across all rules; the factory rejects duplicates.
	Rules []ScopeRule `yaml:"rules"`
}

// Validate validates every rule in the registry configuration.
func (c *ScopeRegistry) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Rules),
	)
}

// DefaultScopeRegistry returns an empty [ScopeRegistry].
func DefaultScopeRegistry() ScopeRegistry {
	return ScopeRegistry{}
}

// IsEnabled reports whether the registry configuration carries any rules.
func (c *ScopeRegistry) IsEnabled() bool {
	return c != nil && len(c.Rules) > 0
}
