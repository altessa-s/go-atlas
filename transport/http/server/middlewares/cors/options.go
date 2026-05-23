// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cors

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/defaults"
)

// Use defaults package for optgen code generation
var _ = defaults.IgnorePatterns

// Default values for CORS configuration.
var (
	// DefaultAllowedMethods are the HTTP methods allowed by default.
	DefaultAllowedMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

	// DefaultAllowedHeaders are the headers allowed by default.
	DefaultAllowedHeaders = []string{
		"Accept",
		"Authorization",
		"Content-Type",
		"Origin",
		"X-Requested-With",
	}

	// DefaultExposedHeaders are the headers exposed by default.
	DefaultExposedHeaders = []string{}

	// DefaultMaxAge is the default max age for preflight cache (24 hours).
	DefaultMaxAge = 86400
)

// options configures the CORS middleware.
type options struct {
	logger         *slog.Logger
	ignorePaths    []string
	ignorePatterns []*regexp.Regexp `optgen:"default=defaults.IgnorePatterns"`

	// allowedOrigins is the list of origins that are allowed to make cross-origin requests.
	// Use "*" or call WithAllowAllOrigins() to allow all origins.
	allowedOrigins []string

	// allowedOriginPatterns is a list of regex patterns for matching allowed origins.
	// This is useful for allowing subdomains dynamically.
	allowedOriginPatterns []*regexp.Regexp

	// allowAllOrigins allows requests from any origin when set to true.
	// WARNING: This is insecure when combined with allowCredentials.
	allowAllOrigins bool

	// allowedMethods is the list of HTTP methods allowed for cross-origin requests.
	allowedMethods []string `optgen:"default=DefaultAllowedMethods"`

	// allowedHeaders is the list of headers that can be used in cross-origin requests.
	allowedHeaders []string `optgen:"default=DefaultAllowedHeaders"`

	// exposedHeaders is the list of headers that browsers are allowed to access.
	exposedHeaders []string `optgen:"default=DefaultExposedHeaders"`

	// allowCredentials indicates whether the request can include credentials
	// like cookies, authorization headers, or TLS client certificates.
	allowCredentials bool

	// maxAge indicates how long (in seconds) the results of a preflight request
	// can be cached. A value of 0 means no caching.
	maxAge int `optgen:"default=DefaultMaxAge"`

	// allowPrivateNetwork enables support for Private Network Access preflight requests.
	// See: https://developer.chrome.com/blog/private-network-access-preflight/
	allowPrivateNetwork bool

	// optionsPassthrough passes OPTIONS requests to the next handler instead of
	// handling them in the middleware.
	optionsPassthrough bool

	// optionsSuccessStatus is the HTTP status code to use for successful OPTIONS requests.
	// Default is 204 No Content.
	optionsSuccessStatus int `optgen:"default=204"`
}
