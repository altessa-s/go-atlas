// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// HttpInterIpAclConfig defines the configuration for the HTTP IP access control middleware.
type HttpInterIpAclConfig struct {
	// BaseHttpMiddlewareConfig provides standard enable and filtering fields.
	BaseHttpMiddlewareConfig `yaml:",inline"`

	// DefaultPolicy determines the default action when an IP matches neither list.
	// "deny" = allowlist mode (deny unless allowed), "allow" = denylist mode (allow unless denied).
	DefaultPolicy string `yaml:"defaultPolicy" default:"deny"`

	// FallbackBehavior defines how the middleware behaves when no valid client IP is available.
	FallbackBehavior FallbackBehavior `yaml:"fallbackBehavior" default:"deny"`

	// Rules defines per-path or per-pattern access control rules.
	Rules []IpAclRuleConfig `yaml:"rules"`

	// DefaultRule is the fallback rule applied when no path-specific rule matches.
	DefaultRule *IpAclRuleConfig `yaml:"defaultRule"`
}

// IsEnabled returns true if IP access control is enabled.
func (c *HttpInterIpAclConfig) IsEnabled() bool {
	return c != nil && c.Enable
}

// Validate performs validation of the HTTP IP access control middleware configuration.
func (c *HttpInterIpAclConfig) Validate() error {
	return c.ValidateBase(func() error {
		return ValidateStruct(c,
			validation.Field(&c.DefaultPolicy, validation.Required,
				ozzo_rules.OneOf("deny", "allow")),
			validation.Field(&c.FallbackBehavior, validation.Required,
				ozzo_rules.OneOf(FallbackBehaviorAllow, FallbackBehaviorDeny, FallbackBehaviorError)),
			validation.Field(&c.Rules, validation.Each(validation.By(validateIpAclRule))),
			validation.Field(&c.DefaultRule, validation.NilOrNotEmpty),
		)
	})
}

// DefaultHttpInterIpAclConfig returns a configuration for IP ACL middleware with default values.
func DefaultHttpInterIpAclConfig() HttpInterIpAclConfig {
	return HttpInterIpAclConfig{
		BaseHttpMiddlewareConfig: BaseHttpMiddlewareConfig{
			EnableMixin: EnableMixin{Enable: false},
		},
		DefaultPolicy:    "deny",
		FallbackBehavior: FallbackBehaviorDeny,
	}
}
