// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"

// RecoveryMiddleware defines the configuration for HTTP panic recovery middleware.
type RecoveryMiddleware struct {
	// BaseMiddleware provides standard enable and filtering fields.
	BaseMiddleware `yaml:",inline"`

	// LogStack enables or disables logging of the stack trace when a panic is recovered.
	// Defaults to true.
	LogStack bool `yaml:"logStack" default:"true"`
}

// Validate performs validation of the RecoveryMiddleware.
func (c *RecoveryMiddleware) Validate() error {
	return c.ValidateBase()
}

// DefaultRecoveryMiddleware returns a configuration for recovery middleware with default values.
func DefaultRecoveryMiddleware() RecoveryMiddleware {
	return RecoveryMiddleware{
		BaseMiddleware: BaseMiddleware{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
		LogStack: true,
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *RecoveryMiddleware) IsEnabled() bool {
	return c != nil && c.Enabled
}
