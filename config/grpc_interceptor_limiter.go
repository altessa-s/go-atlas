// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// GrpcInterLimiterConfig defines the configuration for rate limiting interceptor.
// It controls which requests are subject to rate limiting and provides
// exclusion mechanisms for specific methods or patterns.
type GrpcInterLimiterConfig struct {
	// BaseGrpcInterceptorConfig provides common interceptor configuration.
	BaseGrpcInterceptorConfig `yaml:",inline"`

	// FallbackBehavior defines how the rate limiter behaves when encountering errors
	// or when storage backends are unavailable, providing graceful degradation options.
	// See FallbackBehavior type for available options.
	FallbackBehavior FallbackBehavior `yaml:"fallbackBehavior" default:"deny"`
}

// IsEnabled returns true if rate limiting is enabled.
// This is a convenience method to check if the interceptor should be active.
func (c *GrpcInterLimiterConfig) IsEnabled() bool {
	return c != nil && c.Enable
}

// Validate performs validation of the rate limiter interceptor configuration.
// Ensures that all ignored methods and patterns are properly specified when provided and
// that rate limiting rules and storage are correctly configured.
//
// Validation rules:
//   - IgnoreMethods: each method name must be non-empty when specified
//   - IgnorePatterns: each pattern must be non-empty when specified
//   - FallbackBehavior: must be a valid behavior
//
// Returns an error if validation fails, nil otherwise.
func (c *GrpcInterLimiterConfig) Validate() error {
	return c.ValidateBase(func() error {
		return ValidateStruct(c,
			validation.Field(&c.FallbackBehavior, validation.Required,
				ozzo_rules.OneOf(FallbackBehaviorAllow, FallbackBehaviorDeny, FallbackBehaviorError)),
		)
	})
}

// DefaultGrpcInterLimiterConfig returns a GrpcInterLimiterConfig with default values.
// Rate limiting is disabled by default.
func DefaultGrpcInterLimiterConfig() GrpcInterLimiterConfig {
	return GrpcInterLimiterConfig{
		BaseGrpcInterceptorConfig: BaseGrpcInterceptorConfig{
			EnableMixin: EnableMixin{Enable: false},
		},
		FallbackBehavior: FallbackBehaviorDeny,
	}
}
