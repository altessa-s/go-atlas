// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

// GrpcInterHealthConfig defines the configuration for health check interceptor.
// When enabled, every incoming RPC is preceded by a health check; if the service
// is unhealthy the call is rejected with Unavailable status.
type GrpcInterHealthConfig struct {
	// BaseGrpcInterceptorConfig provides common interceptor configuration.
	BaseGrpcInterceptorConfig `yaml:",inline"`
}

// IsEnabled returns true if the health interceptor is enabled.
func (c *GrpcInterHealthConfig) IsEnabled() bool { return c != nil && c.Enable }

// Validate performs validation of the health interceptor configuration.
//
// Validation rules:
//   - IgnoreMethods: each method name must be non-empty when specified
//   - IgnorePatterns: each pattern must be non-empty when specified
//
// Returns an error if validation fails, nil otherwise.
func (c *GrpcInterHealthConfig) Validate() error {
	return c.ValidateBase()
}

// DefaultGrpcInterHealthConfig returns a GrpcInterHealthConfig with default values.
// Health interceptor is disabled by default.
func DefaultGrpcInterHealthConfig() GrpcInterHealthConfig {
	return GrpcInterHealthConfig{
		BaseGrpcInterceptorConfig: BaseGrpcInterceptorConfig{
			EnableMixin: EnableMixin{Enable: false},
		},
	}
}
