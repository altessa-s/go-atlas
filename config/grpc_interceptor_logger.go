// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"github.com/altessa-s/go-atlas/core/time/timeformat"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// GrpcInterLoggerConfig defines the configuration for request/response logging interceptor.
// Controls what information is logged and how timestamps are formatted.
type GrpcInterLoggerConfig struct {
	// BaseGrpcInterceptorConfig provides common interceptor configuration.
	BaseGrpcInterceptorConfig `yaml:",inline"`

	// TimeFormat defines how timestamps should be formatted in log entries.
	// Must be one of the supported TimeFormat constants.
	// Different formats offer trade-offs between readability and performance.
	TimeFormat timeformat.Format `yaml:"timeFormat" default:"rfc3339"`

	// LogRequest controls whether to log request payloads.
	// Set to true to include request content in logs for debugging purposes.
	LogRequest bool `yaml:"logRequest" default:"false"`

	// LogResponse controls whether to log response payloads.
	// Set to true to include response content in logs for debugging purposes.
	LogResponse bool `yaml:"logResponse" default:"true"`

	// IgnoreGrpcResponseCodes is a list of gRPC response codes to never log.
	// Takes precedence over LogGrpcResponseCodes.
	// Example: [0, 1] to ignore OK and CANCELED codes.
	// Useful for filtering out expected or non-error responses.
	IgnoreGrpcResponseCodes []uint32 `yaml:"ignoreGrpcResponseCodes"`

	// LogGrpcResponseCodes is a list of gRPC response codes to always log.
	// Example: [5, 13] to always log NOT_FOUND and INTERNAL codes.
	// Useful for ensuring important error responses are always captured.
	LogGrpcResponseCodes []uint32 `yaml:"logGrpcResponseCodes"`

	// EnableContextLogger controls whether the logger instance is injected into the context.
	// When enabled, handlers can retrieve the logger using slogx.FromContextOrDefault(ctx).
	EnableContextLogger bool `yaml:"enableContextLogger" default:"false"`
}

// IsEnabled returns true if request/response logging is enabled.
// This is a convenience method to check if the interceptor should be active.
func (c *GrpcInterLoggerConfig) IsEnabled() bool {
	return c != nil && c.Enable
}

// Validate performs validation of the logger interceptor configuration.
// Ensures all fields are properly configured for logging functionality.
//
// Validation rules:
//   - IgnoreMethods: each method name must be non-empty when specified
//   - IgnorePatterns: each pattern must be non-empty when specified
//   - TimeFormat: must be one of the supported timestamp formats
//
// Returns an error if validation fails, nil otherwise.
func (c *GrpcInterLoggerConfig) Validate() error {
	return c.ValidateBase(func() error {
		return ValidateStruct(c,
			validation.Field(&c.TimeFormat,
				ozzo_rules.OneOf(timeformat.RFC3339, timeformat.RFC3339Nano, timeformat.Unix, timeformat.UnixNano, timeformat.UnixMilli)),
		)
	})
}

// DefaultGrpcInterLoggerConfig returns a GrpcInterLoggerConfig with default values.
// Logging is disabled by default.
func DefaultGrpcInterLoggerConfig() GrpcInterLoggerConfig {
	return GrpcInterLoggerConfig{
		BaseGrpcInterceptorConfig: BaseGrpcInterceptorConfig{
			EnableMixin: EnableMixin{Enable: false},
		},
		TimeFormat:          timeformat.RFC3339,
		LogRequest:          false,
		LogResponse:         true,
		EnableContextLogger: false,
	}
}
