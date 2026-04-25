// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// EnableMixin provides a common Enabled field for configurations that can be toggled.
// Embed this struct with `yaml:",inline"` to add an Enabled field to your config.
//
// Note: Each config should still implement its own IsEnabled() method for nil-safety:
//
//	func (c *MyConfig) IsEnabled() bool {
//	    return c != nil && c.Enabled
//	}
//
// Example:
//
//	type MyInterceptorConfig struct {
//	    EnableMixin `yaml:",inline"`
//	    // ... other fields
//	}
type EnableMixin struct {
	// Enabled controls whether this feature is active.
	// When false, the feature is disabled and has no effect.
	Enabled bool `yaml:"enabled" default:"false"`
}

// InterceptorFilterConfig defines common method filtering settings used by interceptors.
// Many gRPC interceptors need to exclude specific methods from processing, either by
// exact method name or by regex pattern matching. This struct provides a reusable
// configuration for such filtering behavior.
//
// When embedded in interceptor configurations, use the `yaml:",inline"` tag to
// flatten the fields in the YAML structure.
//
// Example usage:
//
//	type MyInterceptorConfig struct {
//	    Enabled bool `yaml:"enabled"`
//	    InterceptorFilterConfig `yaml:",inline"`
//	    // ... other fields
//	}
type InterceptorFilterConfig struct {
	// IgnoreMethods is a list of gRPC methods to skip processing for.
	// Methods should be specified in the format "/service.Service/Method".
	// Example: ["/health.Health/Check", "/metrics.Metrics/Get"]
	IgnoreMethods []string `yaml:"ignoreMethods"`

	// IgnorePatterns is a list of regex patterns for methods to skip processing.
	// Patterns are matched against the full method name.
	// Example: ["^/health\\..*", ".*\\.Get$"]
	IgnorePatterns []string `yaml:"ignorePatterns"`
}

// FilterValidationRules returns validation rules for IgnoreMethods and IgnorePatterns fields.
// This method generates validation rules that can be used with ozzo-validation.
//
// Validation rules:
//   - IgnoreMethods: each method name must be non-empty when specified
//   - IgnorePatterns: each pattern must be a valid regex when specified
//
// Example usage:
//
//	func (c *MyInterceptorConfig) Validate() error {
//	    return ValidateStructIfEnabled(c.Enabled, c,
//	        append(c.InterceptorFilterConfig.FilterValidationRules(&c.InterceptorFilterConfig),
//	            validation.Field(&c.OtherField, validation.Required),
//	        )...,
//	    )
//	}
func (c *InterceptorFilterConfig) FilterValidationRules(ptr *InterceptorFilterConfig) []*validation.FieldRules {
	return []*validation.FieldRules{
		validation.Field(&ptr.IgnoreMethods, validation.Each(validation.Required)),
		validation.Field(&ptr.IgnorePatterns, validation.Each(validation.Required, ozzo_rules.Regex())),
	}
}

// FilterValidationRulesBasic returns validation rules without regex validation for patterns.
// Use this when patterns don't need to be valid regexes (just non-empty strings).
//
// Validation rules:
//   - IgnoreMethods: each method name must be non-empty when specified
//   - IgnorePatterns: each pattern must be non-empty when specified
func (c *InterceptorFilterConfig) FilterValidationRulesBasic(ptr *InterceptorFilterConfig) []*validation.FieldRules {
	return []*validation.FieldRules{
		validation.Field(&ptr.IgnoreMethods, validation.Each(validation.Required)),
		validation.Field(&ptr.IgnorePatterns, validation.Each(validation.Required)),
	}
}

// BaseGrpcInterceptorConfig provides common configuration structure for gRPC interceptors.
// It combines EnableMixin and InterceptorFilterConfig with a standard Validate method
// that can be extended with additional validation rules.
//
// Embed this struct in gRPC interceptor configurations to get standard validation behavior:
//
//	type MyInterceptorConfig struct {
//	    BaseGrpcInterceptorConfig `yaml:",inline"`
//	    // ... additional fields
//	}
//
//	func (c *MyInterceptorConfig) Validate() error {
//	    if !c.Enabled {
//	        return nil
//	    }
//	    rules := c.BaseValidationRules()
//	    rules = append(rules,
//	        validation.Field(&c.CustomField, validation.Required),
//	        // ... additional rules
//	    )
//	    return ValidateStruct(c, rules...)
//	}
type BaseGrpcInterceptorConfig struct {
	// EnableMixin controls whether this interceptor is active.
	EnableMixin `yaml:",inline"`

	// InterceptorFilterConfig provides method filtering settings.
	InterceptorFilterConfig `yaml:",inline"`
}

// BaseValidationRules returns the base validation rules for the interceptor.
// Callers should append their own rules and call validation.ValidateStruct themselves.
func (c *BaseGrpcInterceptorConfig) BaseValidationRules() []*validation.FieldRules {
	return append(
		[]*validation.FieldRules{
			validation.Field(&c.Enabled, validation.In(true)),
		},
		c.FilterValidationRulesBasic(&c.InterceptorFilterConfig)...,
	)
}

// ValidateBase performs standard validation for the base interceptor configuration
// and then calls fn for additional validation rules specific to the concrete config.
//
// Returns an error if validation fails, nil otherwise.
func (c *BaseGrpcInterceptorConfig) ValidateBase(fn ...func() error) error {
	if !c.Enabled {
		return nil
	}
	if ret := ValidateStruct(c, c.BaseValidationRules()...); ret != nil {
		return ret
	}
	if len(fn) > 0 {
		return fn[0]()
	}
	return nil
}
