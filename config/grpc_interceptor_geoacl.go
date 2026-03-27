// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"fmt"
	"regexp"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// GeoAclRuleConfig defines a geographic access control rule for one or more endpoints.
type GeoAclRuleConfig struct {
	// Endpoints is a list of exact endpoint names this rule applies to.
	Endpoints []string `yaml:"endpoints"`

	// Patterns is a list of regex patterns for endpoints this rule applies to.
	Patterns []string `yaml:"patterns"`

	// AllowContinents is a list of continent codes that are explicitly allowed.
	AllowContinents []string `yaml:"allowContinents"`

	// DenyContinents is a list of continent codes that are explicitly denied.
	DenyContinents []string `yaml:"denyContinents"`

	// AllowCountries is a list of ISO 3166-1 alpha-2 country codes that are explicitly allowed.
	AllowCountries []string `yaml:"allowCountries"`

	// DenyCountries is a list of ISO 3166-1 alpha-2 country codes that are explicitly denied.
	DenyCountries []string `yaml:"denyCountries"`

	// AllowRegions is a list of region codes (e.g. "US-CA") that are explicitly allowed.
	AllowRegions []string `yaml:"allowRegions"`

	// DenyRegions is a list of region codes (e.g. "US-TX") that are explicitly denied.
	DenyRegions []string `yaml:"denyRegions"`
}

// GrpcInterGeoAclConfig defines the configuration for the geographic access control interceptor.
type GrpcInterGeoAclConfig struct {
	// BaseGrpcInterceptorConfig provides common interceptor configuration.
	BaseGrpcInterceptorConfig `yaml:",inline"`

	// DefaultPolicy determines the default action when a location matches neither list.
	// "deny" = allowlist mode (deny unless allowed), "allow" = denylist mode (allow unless denied).
	DefaultPolicy string `yaml:"defaultPolicy" default:"deny"`

	// FallbackBehavior defines how the interceptor behaves when no valid client IP is available
	// or the geo resolver returns an error.
	FallbackBehavior FallbackBehavior `yaml:"fallbackBehavior" default:"deny"`

	// Rules defines per-endpoint or per-pattern geographic access control rules.
	Rules []GeoAclRuleConfig `yaml:"rules"`

	// DefaultRule is the fallback rule applied when no endpoint-specific rule matches.
	DefaultRule *GeoAclRuleConfig `yaml:"defaultRule"`
}

// IsEnabled returns true if geographic access control is enabled.
func (c *GrpcInterGeoAclConfig) IsEnabled() bool {
	return c != nil && c.Enabled
}

// Validate performs validation of the geographic access control interceptor configuration.
func (c *GrpcInterGeoAclConfig) Validate() error {
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

// DefaultGrpcInterGeoAclConfig returns a GrpcInterGeoAclConfig with default values.
// Geographic access control is disabled by default.
func DefaultGrpcInterGeoAclConfig() GrpcInterGeoAclConfig {
	return GrpcInterGeoAclConfig{
		BaseGrpcInterceptorConfig: BaseGrpcInterceptorConfig{
			EnableMixin: EnableMixin{Enabled: false},
		},
		DefaultPolicy:    "deny",
		FallbackBehavior: FallbackBehaviorDeny,
	}
}

// validContinentCodes is the set of valid 2-letter continent codes.
var validContinentCodes = map[string]struct{}{
	"AF": {}, "AN": {}, "AS": {}, "EU": {}, "NA": {}, "OC": {}, "SA": {},
}

// regionCodePattern matches XX-YY format for region codes.
var regionCodePattern = regexp.MustCompile(`^[A-Z]{2}-[A-Z0-9]{1,3}$`)

// countryCodePattern matches 2-letter uppercase country codes.
var countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)

// validateGeoAclRule validates a single Geo ACL rule entry.
func validateGeoAclRule(value any) error {
	rule, ok := value.(GeoAclRuleConfig)
	if !ok {
		return nil
	}
	return ValidateStruct(&rule,
		validation.Field(&rule.Patterns, validation.Each(validation.Required, ozzo_rules.Regex())),
		validation.Field(&rule.AllowContinents, validation.Each(validation.By(validateContinentCode))),
		validation.Field(&rule.DenyContinents, validation.Each(validation.By(validateContinentCode))),
		validation.Field(&rule.AllowCountries, validation.Each(validation.By(validateCountryCode))),
		validation.Field(&rule.DenyCountries, validation.Each(validation.By(validateCountryCode))),
		validation.Field(&rule.AllowRegions, validation.Each(validation.By(validateRegionCode))),
		validation.Field(&rule.DenyRegions, validation.Each(validation.By(validateRegionCode))),
	)
}

// validateContinentCode validates a continent code is one of: AF, AN, AS, EU, NA, OC, SA.
func validateContinentCode(value any) error {
	s, ok := value.(string)
	if !ok {
		return nil
	}
	if _, valid := validContinentCodes[s]; !valid {
		return fmt.Errorf("invalid continent code %q; must be one of: AF, AN, AS, EU, NA, OC, SA", s)
	}
	return nil
}

// validateCountryCode validates a 2-letter uppercase country code.
func validateCountryCode(value any) error {
	s, ok := value.(string)
	if !ok {
		return nil
	}
	if !countryCodePattern.MatchString(s) {
		return fmt.Errorf("invalid country code %q; must be 2 uppercase letters (ISO 3166-1 alpha-2)", s)
	}
	return nil
}

// validateRegionCode validates a region code in XX-YY format.
func validateRegionCode(value any) error {
	s, ok := value.(string)
	if !ok {
		return nil
	}
	if !regionCodePattern.MatchString(s) {
		return fmt.Errorf("invalid region code %q; must be in XX-YY format (e.g. US-CA)", s)
	}
	return nil
}
