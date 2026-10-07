// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig

import middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"

// AuthInterceptor defines the configuration for authentication interceptor.
// It controls authentication behavior, method exclusions, and caching settings.
type AuthInterceptor struct {
	// BaseInterceptor provides common interceptor configuration.
	BaseInterceptor `yaml:",inline"`
}

// IsEnabled returns true if authentication is enabled.
func (c *AuthInterceptor) IsEnabled() bool { return c != nil && c.Enabled }

// Validate performs validation of the authentication interceptor configuration.
// It ensures all patterns are valid regexes.
//
// Validation rules:
//   - IgnoreMethods: each method name must be non-empty when specified
//   - IgnorePatterns: each pattern must be non-empty when specified
//
// Returns an error if validation fails, nil otherwise.
func (c *AuthInterceptor) Validate() error {
	return c.ValidateBase()
}

// DefaultAuthInterceptor returns a AuthInterceptor with default values.
// Authentication is disabled by default.
func DefaultAuthInterceptor() AuthInterceptor {
	return AuthInterceptor{
		BaseInterceptor: BaseInterceptor{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
	}
}
