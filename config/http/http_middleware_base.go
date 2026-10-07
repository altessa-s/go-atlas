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

// HTTP status code validation bounds.
const (
	httpStatusMin = 100
	httpStatusMax = 599
)

// MiddlewareFilter defines common path filtering settings used by HTTP middlewares.
// Many middlewares need to exclude specific paths from processing, either by
// exact path name or by regex pattern matching. This struct provides a reusable
// configuration for such filtering behavior.
//
// When embedded in middleware configurations, use the `yaml:",inline"` tag to
// flatten the fields in the YAML structure.
//
// Example usage:
//
//	type MyMiddlewareConfig struct {
//	    Enabled bool `yaml:"enabled"`
//	    MiddlewareFilter `yaml:",inline"`
//	    // ... other fields
//	}
type MiddlewareFilter struct {
	// IgnorePaths is a list of exact URL paths to skip processing for.
	// Example: ["/health", "/metrics"]
	IgnorePaths []string `yaml:"ignorePaths"`

	// IgnorePatterns is a list of regex patterns for URL paths to skip processing.
	// Example: ["^/health\\..*", ".*\\.html$"]
	IgnorePatterns []string `yaml:"ignorePatterns"`
}

// FilterValidationRules returns validation rules for IgnorePaths and IgnorePatterns fields.
// This method generates validation rules that can be used with ozzo-validation.
//
// Validation rules:
//   - IgnorePaths: each path must be non-empty when specified
//   - IgnorePatterns: each pattern must be a valid regex when specified
func (c *MiddlewareFilter) FilterValidationRules(ptr *MiddlewareFilter) []*validation.FieldRules {
	return []*validation.FieldRules{
		validation.Field(&ptr.IgnorePaths, validation.Each(validation.Required)),
		validation.Field(&ptr.IgnorePatterns, validation.Each(validation.Required, ozzo_rules.Regex())),
	}
}

// BaseMiddleware provides common configuration structure for HTTP middlewares.
// It combines EnableMixin and MiddlewareFilter with a standard Validate method
// that can be extended with additional validation rules.
//
// Embed this struct in HTTP middleware configurations to get standard validation behavior:
//
//	type MyMiddlewareConfig struct {
//	    BaseMiddleware `yaml:",inline"`
//	    // ... additional fields
//	}
type BaseMiddleware struct {
	// EnableMixin controls whether this middleware is active.
	middlewareconfig.EnableMixin `yaml:",inline"`

	// MiddlewareFilter provides path filtering settings.
	MiddlewareFilter `yaml:",inline"`
}

// ValidateBase performs standard validation for HTTP middleware configurations.
// It validates the Enabled field and filter configuration, then applies additional rules.
//
// Parameters:
//   - additionalRules: extra validation rules specific to the middleware
//
// Returns an error if validation fails, nil otherwise.
func (c *BaseMiddleware) ValidateBase(fn ...func() error) error {
	if !c.Enabled {
		return nil
	}

	// Validate the base configuration fields
	filterRules := c.FilterValidationRules(&c.MiddlewareFilter)
	baseRules := make([]*validation.FieldRules, 0, 1+len(filterRules))
	baseRules = append(baseRules, validation.Field(&c.Enabled, validation.In(true)))

	// Add filter validation rules
	baseRules = append(baseRules, filterRules...)

	if ret := validationconfig.ValidateStruct(c, baseRules...); ret != nil {
		return ret
	}

	if len(fn) > 0 {
		return fn[0]()
	}

	return nil
}
