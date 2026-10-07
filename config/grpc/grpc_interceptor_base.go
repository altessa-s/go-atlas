// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig

import (
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// InterceptorFilter defines common method filtering settings used by interceptors.
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
//	    InterceptorFilter `yaml:",inline"`
//	    // ... other fields
//	}
type InterceptorFilter struct {
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
func (c *InterceptorFilter) FilterValidationRules(ptr *InterceptorFilter) []*validation.FieldRules {
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
func (c *InterceptorFilter) FilterValidationRulesBasic(ptr *InterceptorFilter) []*validation.FieldRules {
	return []*validation.FieldRules{
		validation.Field(&ptr.IgnoreMethods, validation.Each(validation.Required)),
		validation.Field(&ptr.IgnorePatterns, validation.Each(validation.Required)),
	}
}

// BaseInterceptor provides common configuration structure for gRPC interceptors.
// It combines EnableMixin and InterceptorFilter with a standard Validate method
// that can be extended with additional validation rules.
//
// Embed this struct in gRPC interceptor configurations to get standard validation behavior:
//
//	type MyInterceptorConfig struct {
//	    BaseInterceptor `yaml:",inline"`
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
type BaseInterceptor struct {
	// EnableMixin controls whether this interceptor is active.
	middlewareconfig.EnableMixin `yaml:",inline"`

	// InterceptorFilter provides method filtering settings.
	InterceptorFilter `yaml:",inline"`
}

// BaseValidationRules returns the base validation rules for the interceptor.
// Callers should append their own rules and call validation.ValidateStruct themselves.
func (c *BaseInterceptor) BaseValidationRules() []*validation.FieldRules {
	return append(
		[]*validation.FieldRules{
			validation.Field(&c.Enabled, validation.In(true)),
		},
		c.FilterValidationRulesBasic(&c.InterceptorFilter)...,
	)
}

// ValidateBase performs standard validation for the base interceptor configuration
// and then calls fn for additional validation rules specific to the concrete config.
//
// Returns an error if validation fails, nil otherwise.
func (c *BaseInterceptor) ValidateBase(fn ...func() error) error {
	if !c.Enabled {
		return nil
	}
	if ret := validationconfig.ValidateStruct(c, c.BaseValidationRules()...); ret != nil {
		return ret
	}
	if len(fn) > 0 {
		return fn[0]()
	}
	return nil
}
