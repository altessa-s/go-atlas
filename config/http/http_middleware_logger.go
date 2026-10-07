// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import (
	"github.com/altessa-s/go-atlas/core/time/timeformat"

	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// LoggerMiddleware defines the configuration for HTTP request logging middleware.
type LoggerMiddleware struct {
	// BaseMiddleware provides standard enable and filtering fields.
	BaseMiddleware `yaml:",inline"`

	// LogRequest enables or disables logging of the request body.
	// CAUTION: Enabling this may log sensitive data.
	LogRequest bool `yaml:"logRequest" default:"false"`

	// LogResponse enables or disables logging of the response body.
	// CAUTION: Enabling this may log sensitive data.
	LogResponse bool `yaml:"logResponse" default:"false"`

	// TimeFormat defines how timestamps should be formatted in log entries.
	// Must be one of the supported TimeFormat constants.
	TimeFormat timeformat.Format `yaml:"timeFormat" default:"rfc3339"`

	// IgnoreResponseCodes is a list of HTTP status codes that should not be logged.
	// Takes precedence over LogResponseCodes.
	IgnoreResponseCodes []int `yaml:"ignoreResponseCodes"`

	// LogResponseCodes is a list of HTTP status codes that SHOULD be logged.
	// If set, only these codes (minus those in IgnoreResponseCodes) will be logged.
	LogResponseCodes []int `yaml:"logResponseCodes"`

	// IgnoreHttpMethods is a list of HTTP methods to skip logging for.
	// Example: ["OPTIONS", "HEAD"] to skip logging preflight and HEAD requests.
	IgnoreHttpMethods []string `yaml:"ignoreHttpMethods"`
}

// Validate performs validation of the LoggerMiddleware.
func (c *LoggerMiddleware) Validate() error {
	return c.ValidateBase(func() error {
		return validationconfig.ValidateStruct(c,
			validation.Field(&c.TimeFormat,
				ozzo_rules.OneOf(timeformat.RFC3339, timeformat.RFC3339Nano, timeformat.Unix, timeformat.UnixNano, timeformat.UnixMilli)),
			validation.Field(&c.IgnoreResponseCodes, validation.Each(validation.Min(httpStatusMin), validation.Max(httpStatusMax))),
			validation.Field(&c.LogResponseCodes, validation.Each(validation.Min(httpStatusMin), validation.Max(httpStatusMax))),
			validation.Field(&c.IgnoreHttpMethods, validation.Each(validation.Required)),
		)
	})
}

// DefaultLoggerMiddleware returns a configuration for logger middleware with default values.
func DefaultLoggerMiddleware() LoggerMiddleware {
	return LoggerMiddleware{
		BaseMiddleware: BaseMiddleware{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
		TimeFormat:  timeformat.RFC3339,
		LogResponse: true,
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *LoggerMiddleware) IsEnabled() bool {
	return c != nil && c.Enabled
}
