// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

// GrpcInterRecoveryConfig defines the configuration for panic recovery interceptor.
// It controls which methods benefit from panic recovery and provides
// options to disable recovery for specific critical methods.
type GrpcInterRecoveryConfig struct {
	// BaseGrpcInterceptorConfig provides common interceptor configuration.
	BaseGrpcInterceptorConfig `yaml:",inline"`
}

// IsEnabled returns true if panic recovery is enabled.
// This is a convenience method to check if the interceptor should be active.
func (c *GrpcInterRecoveryConfig) IsEnabled() bool {
	return c != nil && c.Enabled
}

// Validate performs validation of the recovery interceptor configuration.
// Ensures that all ignored methods and patterns are properly specified when provided.
//
// Validation rules:
//   - IgnoreMethods: each method name must be non-empty when specified
//   - IgnorePatterns: each pattern must be non-empty when specified
//
// Returns an error if validation fails, nil otherwise.
func (c *GrpcInterRecoveryConfig) Validate() error {
	return c.ValidateBase()
}

// DefaultGrpcInterRecoveryConfig returns a GrpcInterRecoveryConfig with default values.
// Panic recovery is disabled by default.
func DefaultGrpcInterRecoveryConfig() GrpcInterRecoveryConfig {
	return GrpcInterRecoveryConfig{
		BaseGrpcInterceptorConfig: BaseGrpcInterceptorConfig{
			EnableMixin: EnableMixin{Enabled: false},
		},
	}
}
