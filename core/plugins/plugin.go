// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"sync"
	"sync/atomic"
	"time"

	goplugin "plugin"
)

// Plugin represents a loaded .so plugin with its resolved metadata.
//
// Plugin is safe for concurrent use. The mutable fields ([State], [Err]) are
// stored atomically so they can be observed from goroutines other than the
// one calling [Manager.Load]. The immutable identity fields ([Name], [Version],
// [Description], [Path], [LoadedAt]) are set once at construction and never
// modified afterwards, so reading them does not require synchronization.
type Plugin struct {
	// Immutable after construction.
	descriptor Descriptor
	path       string
	raw        *goplugin.Plugin
	loadedAt   time.Time

	// Mutable; accessed concurrently.
	state atomic.Int32           // holds a State value
	err   atomic.Pointer[errBox] // nil pointer means no error

	symbols sync.Map // cached Lookup results: string -> symbolEntry
}

// symbolEntry caches the result of a single symbol lookup.
type symbolEntry struct {
	value any
	ok    bool
}

// errBox wraps an error so it can be stored in an atomic.Pointer.
// Using a wrapper avoids the awkwardness of *error and lets us distinguish
// "not set" (nil pointer) from "explicitly set to nil error" if ever needed.
type errBox struct {
	err error
}

// Name returns the plugin's unique identifier from its descriptor.
func (p *Plugin) Name() string { return p.descriptor.Name }

// Version returns the plugin's version string from its descriptor.
func (p *Plugin) Version() string { return p.descriptor.Version }

// Description returns the plugin's human-readable description.
func (p *Plugin) Description() string { return p.descriptor.Description }

// State returns the current lifecycle state of the plugin.
func (p *Plugin) State() State { return State(p.state.Load()) }

// Path returns the filesystem path of the loaded .so file.
func (p *Plugin) Path() string { return p.path }

// LoadedAt returns the time when the plugin was loaded.
func (p *Plugin) LoadedAt() time.Time { return p.loadedAt }

// Err returns the last error recorded for the plugin (typically when the
// plugin is in [StateFailed]). It returns nil when no error is set.
func (p *Plugin) Err() error {
	if box := p.err.Load(); box != nil {
		return box.err
	}
	return nil
}

// setState atomically updates the plugin lifecycle state.
func (p *Plugin) setState(s State) { p.state.Store(int32(s)) }

// setErr atomically records an error for the plugin.
func (p *Plugin) setErr(err error) { p.err.Store(&errBox{err: err}) }

// Lookup retrieves a symbol exported by the plugin.
//
// Both successful and unsuccessful lookups are cached, so a missing symbol
// is only resolved once per Plugin. Lookup is safe for concurrent use; the
// underlying cache is a [sync.Map].
func (p *Plugin) Lookup(name string) (any, bool) {
	if v, loaded := p.symbols.Load(name); loaded {
		entry := v.(symbolEntry) //nolint:errcheck // internal type, always symbolEntry
		return entry.value, entry.ok
	}

	if p.raw == nil {
		return nil, false
	}

	sym, err := p.raw.Lookup(name)
	entry := symbolEntry{value: sym, ok: err == nil}
	p.symbols.Store(name, entry)
	return entry.value, entry.ok
}
