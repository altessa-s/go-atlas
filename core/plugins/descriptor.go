// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

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
