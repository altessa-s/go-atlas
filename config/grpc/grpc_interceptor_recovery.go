// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig

import middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"

// RecoveryInterceptor defines the configuration for panic recovery interceptor.
// It controls which methods benefit from panic recovery and provides
// options to disable recovery for specific critical methods.
type RecoveryInterceptor struct {
	// BaseInterceptor provides common interceptor configuration.
	BaseInterceptor `yaml:",inline"`
}

// IsEnabled returns true if panic recovery is enabled.
// This is a convenience method to check if the interceptor should be active.
func (c *RecoveryInterceptor) IsEnabled() bool {
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
func (c *RecoveryInterceptor) Validate() error {
	return c.ValidateBase()
}

// DefaultRecoveryInterceptor returns a RecoveryInterceptor with default values.
// Panic recovery is disabled by default.
func DefaultRecoveryInterceptor() RecoveryInterceptor {
	return RecoveryInterceptor{
		BaseInterceptor: BaseInterceptor{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
	}
}
