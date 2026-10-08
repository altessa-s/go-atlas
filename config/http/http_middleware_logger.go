// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import (
	"fmt"
	"log/slog"

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
	IgnoreHttpMethods []string `yaml:"ignoreHTTPMethods"`

	// LogQuery adds the raw query string to each entry as http.query.
	// CAUTION: query strings can carry tokens; list them in RedactQueryParams.
	LogQuery bool `yaml:"logQuery" default:"false"`

	// RedactQueryParams lists query parameter names whose values are logged
	// as REDACTED.
	RedactQueryParams []string `yaml:"redactQueryParams"`

	// StatusLevels overrides the log level per status code, e.g. {401: info}.
	// Levels: debug, info, warn, error. Other codes keep the default level.
	StatusLevels map[int]string `yaml:"statusLevels"`
}

// SlogStatusLevels parses StatusLevels into slog levels.
func (c *LoggerMiddleware) SlogStatusLevels() (map[int]slog.Level, error) {
	if len(c.StatusLevels) == 0 {
		return nil, nil //nolint:nilnil // no overrides
	}
	levels := make(map[int]slog.Level, len(c.StatusLevels))
	for code, name := range c.StatusLevels {
		var level slog.Level
		if err := level.UnmarshalText([]byte(name)); err != nil {
			return nil, fmt.Errorf("statusLevels[%d]: %w", code, err)
		}
		levels[code] = level
	}
	return levels, nil
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
			validation.Field(&c.RedactQueryParams, validation.Each(validation.Required)),
			validation.Field(&c.StatusLevels, validation.By(func(any) error {
				for code := range c.StatusLevels {
					if code < httpStatusMin || code > httpStatusMax {
						return fmt.Errorf("status code %d out of range [%d, %d]", code, httpStatusMin, httpStatusMax)
					}
				}
				_, err := c.SlogStatusLevels()
				return err
			})),
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
