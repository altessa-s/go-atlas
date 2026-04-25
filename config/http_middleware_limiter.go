// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// HttpInterLimiterConfig defines the configuration for HTTP rate limiting middleware.
type HttpInterLimiterConfig struct {
	// BaseHttpMiddlewareConfig provides standard enable and filtering fields.
	BaseHttpMiddlewareConfig `yaml:",inline"`

	// FallbackBehavior determines what happens if the rate limiter itself fails.
	FallbackBehavior FallbackBehavior `yaml:"fallbackBehavior" default:"error"`
}

// Validate performs validation of the HttpInterLimiterConfig.
func (c *HttpInterLimiterConfig) Validate() error {
	return c.ValidateBase(func() error {
		return ValidateStruct(c,
			validation.Field(&c.FallbackBehavior, validation.Required,
				ozzo_rules.OneOf(FallbackBehaviorAllow, FallbackBehaviorDeny, FallbackBehaviorError)),
		)
	})
}

// DefaultHttpInterLimiterConfig returns a configuration for limiter middleware with default values.
func DefaultHttpInterLimiterConfig() HttpInterLimiterConfig {
	return HttpInterLimiterConfig{
		BaseHttpMiddlewareConfig: BaseHttpMiddlewareConfig{
			EnableMixin: EnableMixin{Enabled: false},
		},
		FallbackBehavior: FallbackBehaviorError,
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *HttpInterLimiterConfig) IsEnabled() bool {
	return c != nil && c.Enabled
}
