// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import (
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// LimiterMiddleware defines the configuration for HTTP rate limiting middleware.
type LimiterMiddleware struct {
	// BaseMiddleware provides standard enable and filtering fields.
	BaseMiddleware `yaml:",inline"`

	// FallbackBehavior determines what happens if the rate limiter itself fails.
	FallbackBehavior middlewareconfig.FallbackBehavior `yaml:"fallbackBehavior" default:"error"`
}

// Validate performs validation of the LimiterMiddleware.
func (c *LimiterMiddleware) Validate() error {
	return c.ValidateBase(func() error {
		return validationconfig.ValidateStruct(c,
			validation.Field(&c.FallbackBehavior, validation.Required,
				middlewareconfig.FallbackBehaviorRule()),
		)
	})
}

// DefaultLimiterMiddleware returns a configuration for limiter middleware with default values.
func DefaultLimiterMiddleware() LimiterMiddleware {
	return LimiterMiddleware{
		BaseMiddleware: BaseMiddleware{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
		FallbackBehavior: middlewareconfig.FallbackBehaviorError,
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *LimiterMiddleware) IsEnabled() bool {
	return c != nil && c.Enabled
}
