// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authconfig

import (
	validationconfig "github.com/altessa-s/go-nomos/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// ScopeRule assigns a required scope or public access to a set of action keys.
type ScopeRule struct {
	// Scope is the non-blank scope required for the listed keys.
	Scope string `yaml:"scope"`

	// Public explicitly allows the listed keys once authenticated.
	// Public and Scope are mutually exclusive.
	Public bool `yaml:"public"`

	// Keys are action identifiers such as gRPC full methods
	// ("/pkg.Service/Method"), HTTP route keys, or any stable action id.
	Keys []string `yaml:"keys"`
}

// Validate requires non-blank keys and either a scope or an explicit public rule.
func (r ScopeRule) Validate() error {
	return validationconfig.ValidateStruct(&r,
		validation.Field(&r.Scope, validation.When(r.Public, validation.Empty).Else(validation.By(nonBlankValue))),
		validation.Field(&r.Keys, validation.Required, validation.Each(validation.By(nonBlankValue))),
	)
}

// ScopeRegistry is the configuration for a scope authorization registry: the
// declarative action-key→required-scope table consumed by
// [factory/auth/scope.New]. It carries no principal, matcher, or authorizer
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
//	    - public: true
//	      keys: ["/health.v1.Health/Check"]
type ScopeRegistry struct {
	// Rules is the ordered list of key→scope rules. Each action key must appear
	// at most once across all rules; the factory rejects duplicates.
	Rules []ScopeRule `yaml:"rules"`
}

// Validate validates every rule in the registry configuration.
func (c *ScopeRegistry) Validate() error {
	return validationconfig.ValidateStruct(c,
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
