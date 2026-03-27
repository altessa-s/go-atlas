// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

// HttpInterRecoveryConfig defines the configuration for HTTP panic recovery middleware.
type HttpInterRecoveryConfig struct {
	// BaseHttpMiddlewareConfig provides standard enable and filtering fields.
	BaseHttpMiddlewareConfig `yaml:",inline"`

	// LogStack enables or disables logging of the stack trace when a panic is recovered.
	// Defaults to true.
	LogStack bool `yaml:"logStack" default:"true"`
}

// Validate performs validation of the HttpInterRecoveryConfig.
func (c *HttpInterRecoveryConfig) Validate() error {
	return c.ValidateBase()
}

// DefaultHttpInterRecoveryConfig returns a configuration for recovery middleware with default values.
func DefaultHttpInterRecoveryConfig() HttpInterRecoveryConfig {
	return HttpInterRecoveryConfig{
		BaseHttpMiddlewareConfig: BaseHttpMiddlewareConfig{
			EnableMixin: EnableMixin{Enabled: false},
		},
		LogStack: true,
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *HttpInterRecoveryConfig) IsEnabled() bool {
	return c != nil && c.Enabled
}
