// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import (
	"github.com/altessa-s/go-atlas/internal/transportconfig"

	validationconfig "github.com/altessa-s/go-nomos/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// ErrCredentialedWildcardOrigin indicates that an origin pattern is unsafe with credentials.
var ErrCredentialedWildcardOrigin = transportconfig.ErrCredentialedOriginWildcard

// CORS configuration defaults.
const (
	// DefaultCorsMaxAge is the default max age for preflight cache (24 hours in seconds).
	DefaultCorsMaxAge = 86400

	// DefaultCorsOptionsSuccessStatus is the default HTTP status for successful OPTIONS requests.
	DefaultCorsOptionsSuccessStatus = 204
)

// CORSMiddleware defines the configuration for HTTP Cross-Origin Resource Sharing (CORS) middleware.
type CORSMiddleware struct {
	// BaseMiddleware provides standard enable and filtering fields.
	BaseMiddleware `yaml:",inline"`

	// AllowedOrigins is a list of origins that are allowed to access the resource.
	// Each entry is compared literally.
	AllowedOrigins []string `yaml:"allowedOrigins"`

	// AllowedOriginPatterns is a list of regular expressions for allowed origins.
	// Credentialed patterns must be anchored and confine host wildcards to subdomains of literal registrable domains.
	AllowedOriginPatterns []string `yaml:"allowedOriginPatterns"`

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

// Validate performs validation of the CORSMiddleware.
func (c *CORSMiddleware) Validate() error {
	return c.ValidateBase(func() error {
		return validationconfig.ValidateStruct(c,
			validation.Field(&c.AllowedOrigins, validation.Each(validation.Required)),
			validation.Field(&c.AllowedOriginPatterns, validation.Each(
				validation.By(func(value any) error {
					pattern, _ := value.(string)
					_, err := transportconfig.CompileOriginPattern(pattern, c.AllowCredentials)
					return err
				}))),
			validation.Field(&c.MaxAge, validation.Min(0)),
			validation.Field(&c.OptionsSuccessStatus, validation.Min(httpStatusMin), validation.Max(httpStatusMax)),
		)
	})
}

// DefaultCORSMiddleware returns a configuration for cors middleware with default values.
func DefaultCORSMiddleware() CORSMiddleware {
	return CORSMiddleware{
		Enabled:              false,
		MaxAge:               DefaultCorsMaxAge,
		OptionsSuccessStatus: DefaultCorsOptionsSuccessStatus,
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *CORSMiddleware) IsEnabled() bool {
	return c != nil && c.Enabled
}
