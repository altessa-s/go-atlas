// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

// Default values for HttpPprof configuration.
const (
	defaultHttpPprofEnabled = false
)

// HttpPprof configures pprof profiling endpoints on the HTTP server.
// When enabled, Go's standard pprof handlers are registered under
// the server's internal prefix (default: /internal/pprof/*).
type HttpPprof struct {
	// Enabled determines whether pprof profiling endpoints are exposed.
	// Defaults to false — pprof reveals runtime internals and should only
	// be enabled in non-production environments or behind restricted access.
	Enabled bool `yaml:"enabled" default:"false"`
}

// DefaultHttpPprof returns an HttpPprof configuration with default values.
func DefaultHttpPprof() HttpPprof {
	return HttpPprof{
		Enabled: defaultHttpPprofEnabled,
	}
}

// Validate performs validation on the HttpPprof configuration.
func (p *HttpPprof) Validate() error {
	return nil
}
