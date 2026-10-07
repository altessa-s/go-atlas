// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

// Default values for Pprof configuration.
const (
	defaultHttpPprofEnabled = false
)

// Pprof configures pprof profiling endpoints on the HTTP server.
// When enabled, Go's standard pprof handlers are registered under
// the server's internal prefix (default: /internal/pprof/*).
type Pprof struct {
	// Enabled determines whether pprof profiling endpoints are exposed.
	// Defaults to false — pprof reveals runtime internals and should only
	// be enabled in non-production environments or behind restricted access.
	Enabled bool `yaml:"enabled" default:"false"`
}

// DefaultPprof returns an Pprof configuration with default values.
func DefaultPprof() Pprof {
	return Pprof{
		Enabled: defaultHttpPprofEnabled,
	}
}

// Validate performs validation on the Pprof configuration.
func (p *Pprof) Validate() error {
	return nil
}
