// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import validation "github.com/go-ozzo/ozzo-validation/v4"

// HttpInterTracingConfig defines the configuration for HTTP tracing middleware.
// Controls distributed tracing for HTTP requests.
type HttpInterTracingConfig struct {
	// BaseHttpMiddlewareConfig provides standard enable and filtering fields.
	BaseHttpMiddlewareConfig `yaml:",inline"`

	// RecordBody determines whether request/response bodies are recorded.
	// Warning: This can significantly increase trace size and may expose sensitive data.
	// Defaults to false.
	RecordBody bool `yaml:"recordBody" default:"false"`

	// RecordHeaders determines whether HTTP headers are recorded as span attributes.
	// Sensitive headers (Authorization, Cookie, etc.) are automatically redacted.
	// Defaults to true.
	RecordHeaders bool `yaml:"recordHeaders" default:"true"`

	// RedactedHeaders is a list of header names to redact from traces.
	// Values are replaced with "[REDACTED]".
	// Defaults include Authorization, Cookie, Set-Cookie, X-API-Key.
	RedactedHeaders []string `yaml:"redactedHeaders"`

	// PropagateContext determines whether trace context is propagated from incoming requests.
	// When true, parent spans from incoming requests are linked to server spans.
	// Defaults to true.
	PropagateContext bool `yaml:"propagateContext" default:"true"`

	// RecordClientIP determines whether client IP is recorded as span attribute.
	// Defaults to true.
	RecordClientIP bool `yaml:"recordClientIP" default:"true"`
}

// IsEnabled returns true if the tracing middleware is enabled.
func (c *HttpInterTracingConfig) IsEnabled() bool {
	return c != nil && c.Enable
}

// Validate performs validation of the tracing middleware configuration.
func (c *HttpInterTracingConfig) Validate() error {
	return c.ValidateBase(func() error {
		return ValidateStruct(c,
			validation.Field(&c.RedactedHeaders, validation.Each(validation.Required)),
		)
	})
}

// DefaultHttpInterTracingConfig returns a HttpInterTracingConfig with default values.
// Tracing middleware is disabled by default.
func DefaultHttpInterTracingConfig() HttpInterTracingConfig {
	return HttpInterTracingConfig{
		BaseHttpMiddlewareConfig: BaseHttpMiddlewareConfig{
			EnableMixin: EnableMixin{Enable: false},
		},
		RecordBody:    false,
		RecordHeaders: true,
		RedactedHeaders: []string{
			"Authorization",
			"Cookie",
			"Set-Cookie",
			"X-API-Key",
			"X-Auth-Token",
		},
		PropagateContext: true,
		RecordClientIP:   true,
	}
}
