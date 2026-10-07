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

// IPACLInterceptor defines the configuration for the IP access control interceptor.
type IPACLInterceptor struct {
	// BaseInterceptor provides common interceptor configuration.
	BaseInterceptor `yaml:",inline"`

	// DefaultPolicy determines the default action when an IP matches neither list.
	// "deny" = allowlist mode (deny unless allowed), "allow" = denylist mode (allow unless denied).
	DefaultPolicy string `yaml:"defaultPolicy" default:"deny"`

	// FallbackBehavior defines how the interceptor behaves when no valid client IP is available.
	FallbackBehavior middlewareconfig.FallbackBehavior `yaml:"fallbackBehavior" default:"deny"`

	// Rules defines per-endpoint or per-pattern access control rules.
	Rules []middlewareconfig.IPACLRule `yaml:"rules"`

	// DefaultRule is the fallback rule applied when no endpoint-specific rule matches.
	DefaultRule *middlewareconfig.IPACLRule `yaml:"defaultRule"`
}

// IsEnabled returns true if IP access control is enabled.
func (c *IPACLInterceptor) IsEnabled() bool {
	return c != nil && c.Enabled
}

// Validate performs validation of the IP access control interceptor configuration.
func (c *IPACLInterceptor) Validate() error {
	return c.ValidateBase(func() error {
		return validationconfig.ValidateStruct(c,
			validation.Field(&c.DefaultPolicy, validation.Required,
				ozzo_rules.OneOf("deny", "allow")),
			validation.Field(&c.FallbackBehavior, validation.Required,
				middlewareconfig.FallbackBehaviorRule()),
			validation.Field(&c.Rules, validation.Each(validation.By(middlewareconfig.ValidateIPACLRule))),
			validation.Field(&c.DefaultRule, validation.NilOrNotEmpty),
		)
	})
}

// DefaultIPACLInterceptor returns a IPACLInterceptor with default values.
// IP access control is disabled by default.
func DefaultIPACLInterceptor() IPACLInterceptor {
	return IPACLInterceptor{
		BaseInterceptor: BaseInterceptor{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
		DefaultPolicy:    "deny",
		FallbackBehavior: middlewareconfig.FallbackBehaviorDeny,
	}
}
