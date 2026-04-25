// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import validation "github.com/go-ozzo/ozzo-validation/v4"

// DefaultBodyLimitMaxSize is the default maximum body size (10MB).
const DefaultBodyLimitMaxSize int64 = 10 * 1024 * 1024

// HttpInterBodyLimitConfig defines the configuration for HTTP body size limiting middleware.
type HttpInterBodyLimitConfig struct {
	// BaseHttpMiddlewareConfig provides standard enable and filtering fields.
	BaseHttpMiddlewareConfig `yaml:",inline"`

	// MaxSize is the maximum allowed size of the request body in bytes.
	// Defaults to 10MB (10485760 bytes).
	MaxSize int64 `yaml:"maxSize" default:"10485760"`
}

// Validate performs validation of the HttpInterBodyLimitConfig.
func (c *HttpInterBodyLimitConfig) Validate() error {
	return c.ValidateBase(func() error {
		return ValidateStruct(c,
			validation.Field(&c.MaxSize, validation.Min(0)),
		)
	})
}

// DefaultHttpInterBodyLimitConfig returns a configuration for bodylimit middleware with default values.
func DefaultHttpInterBodyLimitConfig() HttpInterBodyLimitConfig {
	return HttpInterBodyLimitConfig{
		BaseHttpMiddlewareConfig: BaseHttpMiddlewareConfig{
			EnableMixin: EnableMixin{Enabled: false},
		},
		MaxSize: DefaultBodyLimitMaxSize,
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *HttpInterBodyLimitConfig) IsEnabled() bool {
	return c != nil && c.Enabled
}
