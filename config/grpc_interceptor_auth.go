// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

// GrpcInterAuthConfig defines the configuration for authentication interceptor.
// It controls authentication behavior, method exclusions, and caching settings.
type GrpcInterAuthConfig struct {
	// BaseGrpcInterceptorConfig provides common interceptor configuration.
	BaseGrpcInterceptorConfig `yaml:",inline"`
}

// IsEnabled returns true if authentication is enabled.
func (c *GrpcInterAuthConfig) IsEnabled() bool { return c != nil && c.Enabled }

// Validate performs validation of the authentication interceptor configuration.
// It ensures all patterns are valid regexes.
//
// Validation rules:
//   - IgnoreMethods: each method name must be non-empty when specified
//   - IgnorePatterns: each pattern must be non-empty when specified
//
// Returns an error if validation fails, nil otherwise.
func (c *GrpcInterAuthConfig) Validate() error {
	return c.ValidateBase()
}

// DefaultGrpcInterAuthConfig returns a GrpcInterAuthConfig with default values.
// Authentication is disabled by default.
func DefaultGrpcInterAuthConfig() GrpcInterAuthConfig {
	return GrpcInterAuthConfig{
		BaseGrpcInterceptorConfig: BaseGrpcInterceptorConfig{
			EnableMixin: EnableMixin{Enabled: false},
		},
	}
}
