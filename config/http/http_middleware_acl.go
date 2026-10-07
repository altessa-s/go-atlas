// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import (
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// ACLMiddleware is a generic base for HTTP ACL middleware configurations.
// R is the rule type (e.g. GeoAclRuleConfig or IpAclRuleConfig).
type ACLMiddleware[R any] struct {
	// BaseMiddleware provides standard enable and filtering fields.
	BaseMiddleware `yaml:",inline"`

	// DefaultPolicy determines the default action when a request matches neither list.
	// "deny" = allowlist mode (deny unless allowed), "allow" = denylist mode (allow unless denied).
	DefaultPolicy string `yaml:"defaultPolicy" default:"deny"`

	// FallbackBehavior defines how the middleware behaves when no valid client IP is available
	// or the resolver returns an error.
	FallbackBehavior middlewareconfig.FallbackBehavior `yaml:"fallbackBehavior" default:"deny"`

	// Rules defines per-path or per-pattern access control rules.
	Rules []R `yaml:"rules"`

	// DefaultRule is the fallback rule applied when no path-specific rule matches.
	DefaultRule *R `yaml:"defaultRule"`
}

// IsEnabled returns true if the ACL middleware is enabled.
func (c *ACLMiddleware[R]) IsEnabled() bool {
	return c != nil && c.Enabled
}

// ValidateWith validates common fields, delegating rule validation to ruleValidator.
func (c *ACLMiddleware[R]) ValidateWith(ruleValidator func(any) error) error {
	return c.ValidateBase(func() error {
		return validationconfig.ValidateStruct(c,
			validation.Field(&c.DefaultPolicy, validation.Required,
				ozzo_rules.OneOf("deny", "allow")),
			validation.Field(&c.FallbackBehavior, validation.Required,
				middlewareconfig.FallbackBehaviorRule()),
			validation.Field(&c.Rules, validation.Each(validation.By(ruleValidator))),
			validation.Field(&c.DefaultRule, validation.NilOrNotEmpty),
		)
	})
}

// DefaultACLMiddleware returns a default ACL middleware configuration.
func DefaultACLMiddleware[R any]() ACLMiddleware[R] {
	return ACLMiddleware[R]{
		BaseMiddleware: BaseMiddleware{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
		DefaultPolicy:    "deny",
		FallbackBehavior: middlewareconfig.FallbackBehaviorDeny,
	}
}
