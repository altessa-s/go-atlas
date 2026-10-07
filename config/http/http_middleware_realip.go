// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import (
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// RealIPMiddleware defines the configuration for HTTP real IP extraction middleware.
type RealIPMiddleware struct {
	// BaseMiddleware provides standard enable and filtering fields.
	BaseMiddleware `yaml:",inline"`

	// Headers is a list of HTTP headers to check for the real client IP.
	// Headers are checked in the order specified.
	// Common values: ["X-Forwarded-For", "X-Real-IP", "True-Client-IP"].
	Headers []string `yaml:"headers"`

	// TrustedProxies is a list of IP addresses or CIDR ranges of trusted proxies.
	// Only IP addresses from these proxies will be considered when extracting IP from headers.
	TrustedProxies []string `yaml:"trustedProxies"`
}

// Validate performs validation of the RealIPMiddleware.
func (c *RealIPMiddleware) Validate() error {
	return c.ValidateBase(func() error {
		return validationconfig.ValidateStruct(c,
			validation.Field(&c.Headers, validation.Each(validation.Required)),
			validation.Field(&c.TrustedProxies, validation.Each(validation.Required)),
		)
	})
}

// DefaultRealIPMiddleware returns a configuration for realip middleware with default values.
func DefaultRealIPMiddleware() RealIPMiddleware {
	return RealIPMiddleware{
		BaseMiddleware: BaseMiddleware{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
		Headers: []string{"X-Forwarded-For", "X-Real-IP"},
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *RealIPMiddleware) IsEnabled() bool {
	return c != nil && c.Enabled
}
