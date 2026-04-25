// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import validation "github.com/go-ozzo/ozzo-validation/v4"

// HttpInterRequestIdConfig defines the configuration for HTTP request ID middleware.
type HttpInterRequestIdConfig struct {
	// BaseHttpMiddlewareConfig provides standard enable and filtering fields.
	BaseHttpMiddlewareConfig `yaml:",inline"`

	// HeaderName is the name of the HTTP header used to pass/return the request ID.
	// Defaults to "X-Request-ID".
	HeaderName string `yaml:"headerName" default:"X-Request-ID"`

	// GenerateIfMissing determines whether to generate a new request ID if none is provided.
	// Defaults to true.
	GenerateIfMissing bool `yaml:"generateIfMissing" default:"true"`
}

// Validate performs validation of the HttpInterRequestIdConfig.
func (c *HttpInterRequestIdConfig) Validate() error {
	return c.ValidateBase(func() error {
		return ValidateStruct(c, validation.Field(&c.HeaderName, validation.Required))
	})
}

// DefaultHttpInterRequestIdConfig returns a configuration for requestid middleware with default values.
func DefaultHttpInterRequestIdConfig() HttpInterRequestIdConfig {
	return HttpInterRequestIdConfig{
		BaseHttpMiddlewareConfig: BaseHttpMiddlewareConfig{
			EnableMixin: EnableMixin{Enabled: false},
		},
		HeaderName:        "X-Request-ID",
		GenerateIfMissing: true,
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *HttpInterRequestIdConfig) IsEnabled() bool {
	return c != nil && c.Enabled
}
