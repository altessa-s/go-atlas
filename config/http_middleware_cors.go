// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import validation "github.com/go-ozzo/ozzo-validation/v4"

// CORS configuration defaults.
const (
	// DefaultCorsMaxAge is the default max age for preflight cache (24 hours in seconds).
	DefaultCorsMaxAge = 86400

	// DefaultCorsOptionsSuccessStatus is the default HTTP status for successful OPTIONS requests.
	DefaultCorsOptionsSuccessStatus = 204
)

// HttpInterCorsConfig defines the configuration for HTTP Cross-Origin Resource Sharing (CORS) middleware.
type HttpInterCorsConfig struct {
	// BaseHttpMiddlewareConfig provides standard enable and filtering fields.
	BaseHttpMiddlewareConfig `yaml:",inline"`

	// AllowedOrigins is a list of origins that are allowed to access the resource.
	// Use ["*"] to allow all origins.
	AllowedOrigins []string `yaml:"allowedOrigins"`

	// AllowedMethods is a list of HTTP methods allowed when accessing the resource.
	// Defaults to standard methods if not specified.
	AllowedMethods []string `yaml:"allowedMethods"`

	// AllowedHeaders is a list of request headers that can be used when making the actual request.
	AllowedHeaders []string `yaml:"allowedHeaders"`

	// ExposedHeaders is a whitelist of headers that browsers are allowed to access.
	ExposedHeaders []string `yaml:"exposedHeaders"`

	// AllowCredentials indicates whether the request can include user credentials like cookies or auth headers.
	AllowCredentials bool `yaml:"allowCredentials" default:"false"`

	// MaxAge indicates how long (in seconds) the results of a preflight request can be cached.
	MaxAge int `yaml:"maxAge" default:"86400"`

	// AllowPrivateNetwork enables support for Private Network Access preflight requests.
	AllowPrivateNetwork bool `yaml:"allowPrivateNetwork" default:"false"`

	// OptionsPassthrough passes OPTIONS requests to the next handler instead of handling them internally.
	OptionsPassthrough bool `yaml:"optionsPassthrough" default:"false"`

	// OptionsSuccessStatus is the status code to return for successful OPTIONS requests.
	// Defaults to 204.
	OptionsSuccessStatus int `yaml:"optionsSuccessStatus" default:"204"`
}

// Validate performs validation of the HttpInterCorsConfig.
func (c *HttpInterCorsConfig) Validate() error {
	return c.ValidateBase(func() error {
		return ValidateStruct(c,
			validation.Field(&c.AllowedOrigins, validation.Each(validation.Required)),
			validation.Field(&c.MaxAge, validation.Min(0)),
			validation.Field(&c.OptionsSuccessStatus, validation.Min(httpStatusMin), validation.Max(httpStatusMax)),
		)
	})
}

// DefaultHttpInterCorsConfig returns a configuration for cors middleware with default values.
func DefaultHttpInterCorsConfig() HttpInterCorsConfig {
	return HttpInterCorsConfig{
		BaseHttpMiddlewareConfig: BaseHttpMiddlewareConfig{
			EnableMixin: EnableMixin{Enabled: false},
		},
		MaxAge:               DefaultCorsMaxAge,
		OptionsSuccessStatus: DefaultCorsOptionsSuccessStatus,
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *HttpInterCorsConfig) IsEnabled() bool {
	return c != nil && c.Enabled
}
