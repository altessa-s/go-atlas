// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig //nolint:dupl // IP and geo ACL schemas are parallel by design; their rule types differ.

import (
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// GeoACLInterceptor defines the configuration for the geographic access control interceptor.
type GeoACLInterceptor struct {
	// BaseInterceptor provides common interceptor configuration.
	BaseInterceptor `yaml:",inline"`

	// DefaultPolicy determines the default action when a location matches neither list.
	// "deny" = allowlist mode (deny unless allowed), "allow" = denylist mode (allow unless denied).
	DefaultPolicy string `yaml:"defaultPolicy" default:"deny"`

	// FallbackBehavior defines how the interceptor behaves when no valid client IP is available
	// or the geo resolver returns an error.
	FallbackBehavior middlewareconfig.FallbackBehavior `yaml:"fallbackBehavior" default:"deny"`

	// Rules defines per-endpoint or per-pattern geographic access control rules.
	Rules []middlewareconfig.GeoACLRule `yaml:"rules"`

	// DefaultRule is the fallback rule applied when no endpoint-specific rule matches.
	DefaultRule *middlewareconfig.GeoACLRule `yaml:"defaultRule"`
}

// IsEnabled returns true if geographic access control is enabled.
func (c *GeoACLInterceptor) IsEnabled() bool {
	return c != nil && c.Enabled
}

// Validate performs validation of the geographic access control interceptor configuration.
func (c *GeoACLInterceptor) Validate() error {
	return c.ValidateBase(func() error {
		return validationconfig.ValidateStruct(c,
			validation.Field(&c.DefaultPolicy, validation.Required,
				ozzo_rules.OneOf("deny", "allow")),
			validation.Field(&c.FallbackBehavior, validation.Required,
				middlewareconfig.FallbackBehaviorRule()),
			validation.Field(&c.Rules, validation.Each(validation.By(middlewareconfig.ValidateGeoACLRule))),
			validation.Field(&c.DefaultRule, validation.NilOrNotEmpty),
		)
	})
}

// DefaultGeoACLInterceptor returns a GeoACLInterceptor with default values.
// Geographic access control is disabled by default.
func DefaultGeoACLInterceptor() GeoACLInterceptor {
	return GeoACLInterceptor{
		BaseInterceptor: BaseInterceptor{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
		DefaultPolicy:    "deny",
		FallbackBehavior: middlewareconfig.FallbackBehaviorDeny,
	}
}
