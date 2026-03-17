// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// HttpInterGeoAclConfig defines the configuration for the HTTP geographic access control middleware.
type HttpInterGeoAclConfig struct {
	// BaseHttpMiddlewareConfig provides standard enable and filtering fields.
	BaseHttpMiddlewareConfig `yaml:",inline"`

	// DefaultPolicy determines the default action when a location matches neither list.
	// "deny" = allowlist mode (deny unless allowed), "allow" = denylist mode (allow unless denied).
	DefaultPolicy string `yaml:"defaultPolicy" default:"deny"`

	// FallbackBehavior defines how the middleware behaves when no valid client IP is available
	// or the geo resolver returns an error.
	FallbackBehavior FallbackBehavior `yaml:"fallbackBehavior" default:"deny"`

	// Rules defines per-path or per-pattern geographic access control rules.
	Rules []GeoAclRuleConfig `yaml:"rules"`

	// DefaultRule is the fallback rule applied when no path-specific rule matches.
	DefaultRule *GeoAclRuleConfig `yaml:"defaultRule"`
}

// IsEnabled returns true if geographic access control is enabled.
func (c *HttpInterGeoAclConfig) IsEnabled() bool {
	return c != nil && c.Enable
}

// Validate performs validation of the HTTP geographic access control middleware configuration.
func (c *HttpInterGeoAclConfig) Validate() error {
	return c.ValidateBase(func() error {
		return ValidateStruct(c,
			validation.Field(&c.DefaultPolicy, validation.Required,
				ozzo_rules.OneOf("deny", "allow")),
			validation.Field(&c.FallbackBehavior, validation.Required,
				ozzo_rules.OneOf(FallbackBehaviorAllow, FallbackBehaviorDeny, FallbackBehaviorError)),
			validation.Field(&c.Rules, validation.Each(validation.By(validateGeoAclRule))),
			validation.Field(&c.DefaultRule, validation.NilOrNotEmpty),
		)
	})
}

// DefaultHttpInterGeoAclConfig returns a configuration for GeoACL middleware with default values.
func DefaultHttpInterGeoAclConfig() HttpInterGeoAclConfig {
	return HttpInterGeoAclConfig{
		BaseHttpMiddlewareConfig: BaseHttpMiddlewareConfig{
			EnableMixin: EnableMixin{Enable: false},
		},
		DefaultPolicy:    "deny",
		FallbackBehavior: FallbackBehaviorDeny,
	}
}
