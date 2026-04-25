// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Descriptor carries metadata that every plugin must export. The .so file
// must have a package-level variable named Descriptor; either the value or
// pointer declaration form is accepted:
//
//	var Descriptor = plugins.Descriptor{
//	    Name:    "my-plugin",
//	    Version: "1.0.0",
//	}
//
//	var Descriptor = &plugins.Descriptor{
//	    Name:    "my-plugin",
//	    Version: "1.0.0",
//	}
//
// The manager unwraps both shapes through [plugin.Lookup] — see the package
// overview in [plugins] for the full plugin contract.
type Descriptor struct {
	// Name is the unique identifier for the plugin.
	Name string

	// Version is a semver string for the plugin.
	Version string

	// Description is optional human-readable text.
	Description string

	// GoVersion is the Go toolchain version the plugin was built with
	// (e.g. "go1.25.0"). When non-empty the manager compares it against
	// [runtime.Version] at load time and logs a warning on mismatch.
	// The field is advisory — a mismatch does not prevent loading because
	// [plugin.Open] itself enforces the real ABI check. The warning gives
	// operators a clear diagnostic before the cryptic runtime error.
	GoVersion string
}

// Validate reports whether the descriptor is well-formed enough for the
// manager to register it. Currently this only enforces that Name is
// non-empty — Version and Description are operator metadata and the
// manager does not depend on them for routing or invariants.
//
// Errors are wrapped in [ErrInvalidDescriptor] so callers can match
// programmatically via [errors.Is] regardless of the underlying
// validation rule that failed. The plugin loader runs Validate as part
// of [resolveDescriptor] before any plugin is registered; operators
// who construct a Descriptor programmatically (e.g. for tests) can
// also call it directly to fail fast.
func (d Descriptor) Validate() error {
	if d.Name == "" {
		return coreerrs.Wrap(ErrInvalidDescriptor, "Descriptor.Name is empty")
	}
	return nil
}

// State represents the lifecycle state of a loaded plugin.
type State int

const (
	// StateLoaded means the .so was opened and the descriptor was resolved,
	// but Init has not yet been called.
	StateLoaded State = iota

	// StateReady means the plugin's Init completed successfully
	// (or was not present). The plugin is fully operational.
	StateReady

	// StateFailed means the plugin's Init returned an error or panicked.
	// The plugin is not operational; see [Plugin.Err] for details.
	StateFailed

	// StateUnloaded means the plugin was explicitly removed from the manager.
	StateUnloaded
)

// String returns a human-readable representation of the state.
func (s State) String() string {
	switch s {
	case StateLoaded:
		return "loaded"
	case StateReady:
		return "ready"
	case StateFailed:
		return "failed"
	case StateUnloaded:
		return "unloaded"
	default:
		return "unknown"
	}
}
