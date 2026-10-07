// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import (
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// DefaultBodyLimitMaxSize is the default maximum body size (10MB).
const DefaultBodyLimitMaxSize int64 = 10 * 1024 * 1024

// BodyLimitMiddleware defines the configuration for HTTP body size limiting middleware.
type BodyLimitMiddleware struct {
	// BaseMiddleware provides standard enable and filtering fields.
	BaseMiddleware `yaml:",inline"`

	// MaxSize is the maximum allowed size of the request body in bytes.
	// Defaults to 10MB (10485760 bytes).
	MaxSize int64 `yaml:"maxSize" default:"10485760"`

	// RequireContentLength rejects body-bearing requests
	// (POST/PUT/PATCH/DELETE) that omit Content-Length with
	// 411 Length Required. Closes a chunked-body bypass: without it,
	// Transfer-Encoding: chunked sets ContentLength=-1 and slips past
	// the MaxSize pre-check until the handler reads the body — an
	// attacker can then drip chunks for the full readTimeout window
	// if a handler ignores its body.
	//
	// The YAML default is true: production deployments rarely need
	// chunked uploads, and leaving the bypass open is worse than
	// rejecting a few legitimate streaming clients. The matching Go
	// API option [bodylimit.WithRequireContentLength] is opt-in and
	// stays off by default for backward compatibility with existing
	// programmatic callers.
	RequireContentLength bool `yaml:"requireContentLength" default:"true"`
}

// Validate performs validation of the BodyLimitMiddleware.
func (c *BodyLimitMiddleware) Validate() error {
	return c.ValidateBase(func() error {
		return validationconfig.ValidateStruct(c,
			validation.Field(&c.MaxSize, validation.Min(0)),
		)
	})
}

// DefaultBodyLimitMiddleware returns a configuration for bodylimit middleware with default values.
func DefaultBodyLimitMiddleware() BodyLimitMiddleware {
	return BodyLimitMiddleware{
		BaseMiddleware: BaseMiddleware{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
		MaxSize:              DefaultBodyLimitMaxSize,
		RequireContentLength: true,
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *BodyLimitMiddleware) IsEnabled() bool {
	return c != nil && c.Enabled
}
