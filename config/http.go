// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Http configuration.
const (
	defaultHttpListenAddress     = "0.0.0.0:9080"
	defaultHttpReadTimeout       = 300 * time.Second
	defaultHttpReadHeaderTimeout = 10 * time.Second
	defaultHttpWriteTimeout      = 300 * time.Second
	defaultHttpIdleTimeout       = 600 * time.Second
	defaultHttpLogRequests       = false
)

// Http represents the configuration for HTTP server settings.
// It contains all necessary parameters for configuring an HTTP server including
// timeouts, payload limits, and optional TLS configuration.
//
// Example:
//
//	httpCfg := &config.Http{
//		ListenAddress: "0.0.0.0:8080",
//		ReadTimeout:   30 * time.Second,
//	}
type Http struct {
	// RouterPrefix is an optional prefix for all Http routes.
	// Allows mounting the service under a specific path prefix.
	// If empty, routes are mounted at the root level.
	RouterPrefix string `yaml:"routerPrefix"`

	// ListenAddress specifies the address and port to bind the Http server to.
	// Defaults to "0.0.0.0:9080" which binds to all interfaces on port 9080.
	ListenAddress string `yaml:"listenAddress" default:"0.0.0.0:9080"`

	// ReadTimeout is the maximum duration for reading the entire request,
	// including the body. Defaults to 300 seconds.
	ReadTimeout time.Duration `yaml:"readTimeout" default:"300s"`

	// ReadHeaderTimeout caps the time the server spends reading the
	// request headers. Independent from ReadTimeout, this is the primary
	// Slowloris defense: it bounds the header-read phase to a short window
	// even when ReadTimeout is intentionally long (e.g. for streaming
	// uploads). Defaults to 10 seconds.
	ReadHeaderTimeout time.Duration `yaml:"readHeaderTimeout" default:"10s"`

	// WriteTimeout is the maximum duration before timing out writes of the response.
	// Defaults to 300 seconds.
	WriteTimeout time.Duration `yaml:"writeTimeout" default:"300s"`

	// IdleTimeout is the maximum amount of time to wait for the next request
	// when keep-alives are enabled. Defaults to 600 seconds.
	IdleTimeout time.Duration `yaml:"idleTimeout" default:"600s"`

	// LogRequests enables or disables Http request logging.
	// Defaults to false.
	LogRequests bool `yaml:"logRequests" default:"false"`

	// Tls contains optional Tls configuration for HTTPS.
	// If nil, the server will run in Http mode only.
	TLS *HttpTls `yaml:"tls" default:"-"`

	// Middlewares contains configuration for HTTP server middleware components.
	Middlewares *MiddlewaresConfig `yaml:"middlewares" default:"-"`

	// Pprof contains optional configuration for pprof profiling endpoints.
	// If nil, pprof behavior is determined by the server builder defaults.
	Pprof *HttpPprof `yaml:"pprof" default:"-"`
}

// DefaultHttp returns an Http configuration with default values.
func DefaultHttp() Http {
	return Http{
		ListenAddress:     defaultHttpListenAddress,
		ReadTimeout:       defaultHttpReadTimeout,
		ReadHeaderTimeout: defaultHttpReadHeaderTimeout,
		WriteTimeout:      defaultHttpWriteTimeout,
		IdleTimeout:       defaultHttpIdleTimeout,
		LogRequests:       defaultHttpLogRequests,
	}
}

// Validate performs validation on the Http configuration.
// It validates the listen address format, timeout durations, and Tls
// configuration if present.
//
// Returns an error if any validation rules fail.
func (h *Http) Validate() error {
	return ValidateStruct(h,
		validation.Field(&h.ListenAddress, ozzo_rules.ListenAddress()),
		validation.Field(&h.ReadTimeout, ozzo_rules.DurationOrZero()),
		validation.Field(&h.ReadHeaderTimeout, ozzo_rules.DurationOrZero()),
		validation.Field(&h.WriteTimeout, ozzo_rules.DurationOrZero()),
		validation.Field(&h.IdleTimeout, ozzo_rules.DurationOrZero()),
		validation.Field(&h.TLS, validation.NilOrNotEmpty),
		validation.Field(&h.Middlewares, validation.NilOrNotEmpty),
		validation.Field(&h.Pprof, validation.NilOrNotEmpty),
	)
}
