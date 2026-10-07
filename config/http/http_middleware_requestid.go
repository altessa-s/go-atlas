// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import (
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// RequestIDMiddleware defines the configuration for HTTP request ID middleware.
type RequestIDMiddleware struct {
	// BaseMiddleware provides standard enable and filtering fields.
	BaseMiddleware `yaml:",inline"`

	// HeaderName is the name of the HTTP header used to pass/return the request ID.
	// Defaults to "X-Request-ID".
	HeaderName string `yaml:"headerName" default:"X-Request-ID"`

	// GenerateIfMissing determines whether to generate a new request ID if none is provided.
	// Defaults to true.
	GenerateIfMissing bool `yaml:"generateIfMissing" default:"true"`
}

// Validate performs validation of the RequestIDMiddleware.
func (c *RequestIDMiddleware) Validate() error {
	return c.ValidateBase(func() error {
		return validationconfig.ValidateStruct(c, validation.Field(&c.HeaderName, validation.Required))
	})
}

// DefaultRequestIDMiddleware returns a configuration for requestid middleware with default values.
func DefaultRequestIDMiddleware() RequestIDMiddleware {
	return RequestIDMiddleware{
		BaseMiddleware: BaseMiddleware{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
		HeaderName:        "X-Request-ID",
		GenerateIfMissing: true,
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *RequestIDMiddleware) IsEnabled() bool {
	return c != nil && c.Enabled
}
