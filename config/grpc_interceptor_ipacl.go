// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// IpAclRuleConfig defines an IP access control rule for one or more endpoints.
type IpAclRuleConfig struct {
	// Endpoints is a list of exact endpoint names this rule applies to.
	Endpoints []string `yaml:"endpoints"`

	// Patterns is a list of regex patterns for endpoints this rule applies to.
	Patterns []string `yaml:"patterns"`

	// Allowlist is a list of CIDR prefixes that are explicitly allowed.
	Allowlist []string `yaml:"allowlist"`

	// Denylist is a list of CIDR prefixes that are explicitly denied.
	Denylist []string `yaml:"denylist"`
}

// GrpcInterIpAclConfig defines the configuration for the IP access control interceptor.
type GrpcInterIpAclConfig struct {
	// BaseGrpcInterceptorConfig provides common interceptor configuration.
	BaseGrpcInterceptorConfig `yaml:",inline"`

	// DefaultPolicy determines the default action when an IP matches neither list.
	// "deny" = allowlist mode (deny unless allowed), "allow" = denylist mode (allow unless denied).
	DefaultPolicy string `yaml:"defaultPolicy" default:"deny"`

	// FallbackBehavior defines how the interceptor behaves when no valid client IP is available.
	FallbackBehavior FallbackBehavior `yaml:"fallbackBehavior" default:"deny"`

	// Rules defines per-endpoint or per-pattern access control rules.
	Rules []IpAclRuleConfig `yaml:"rules"`

	// DefaultRule is the fallback rule applied when no endpoint-specific rule matches.
	DefaultRule *IpAclRuleConfig `yaml:"defaultRule"`
}

// IsEnabled returns true if IP access control is enabled.
func (c *GrpcInterIpAclConfig) IsEnabled() bool {
	return c != nil && c.Enabled
}

// Validate performs validation of the IP access control interceptor configuration.
func (c *GrpcInterIpAclConfig) Validate() error {
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

// DefaultGrpcInterIpAclConfig returns a GrpcInterIpAclConfig with default values.
// IP access control is disabled by default.
func DefaultGrpcInterIpAclConfig() GrpcInterIpAclConfig {
	return GrpcInterIpAclConfig{
		BaseGrpcInterceptorConfig: BaseGrpcInterceptorConfig{
			EnableMixin: EnableMixin{Enabled: false},
		},
		DefaultPolicy:    "deny",
		FallbackBehavior: FallbackBehaviorDeny,
	}
}

// validateIpAclRule validates a single IP ACL rule entry.
func validateIpAclRule(value any) error {
	rule, ok := value.(IpAclRuleConfig)
	if !ok {
		return nil
	}
	return ValidateStruct(&rule,
		validation.Field(&rule.Patterns, validation.Each(validation.Required, ozzo_rules.Regex())),
		validation.Field(&rule.Allowlist, validation.Each(validation.By(validateIP))),
		validation.Field(&rule.Denylist, validation.Each(validation.By(validateIP))),
	)
}
