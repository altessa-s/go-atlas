// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"fmt"
	"log/slog"
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

// newPlugin builds a [*Plugin] in the canonical [StateLoaded] entry state
// from a resolved descriptor and the underlying *plugin.Plugin handle.
//
// Centralizing construction here keeps two invariants enforceable:
//
//   - Every required identity field (descriptor, path, raw) is set in one
//     place, so a future caller cannot accidentally build a Plugin with a
//     zero-value Descriptor and a stale path.
//   - The initial State is set explicitly via [Plugin.setState] rather
//     than relying on the coincidence that StateLoaded happens to equal
//     the zero value of int32. A change to the State iota order would
//     otherwise silently break every newly-constructed plugin's reported
//     state until the first transition.
//
// raw may be nil for tests that exercise the manager registry without a
// real .so file; production callers always pass a non-nil handle from
// [openPlugin].
func newPlugin(desc Descriptor, path string, raw *goplugin.Plugin) *Plugin {
	p := &Plugin{
		descriptor: desc,
		path:       path,
		raw:        raw,
		loadedAt:   time.Now(),
	}
	p.setState(StateLoaded)
	return p
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
//
// The valid transitions are:
//
//	StateLoaded   → StateReady | StateFailed | StateUnloaded
//	StateReady    → StateFailed | StateUnloaded
//	StateFailed   → StateUnloaded
//	StateUnloaded → (terminal)
//
// Invalid transitions are logged at WARN with a stack-frame hint and
// then applied unconditionally — the package is internally consistent
// today, and a refusal-to-set could leave a plugin observably stuck in
// the wrong state, which is worse than a noisy log line. The log
// pattern surfaces refactor regressions in tests and CI without
// breaking production.
//
// In tests this can be made stricter via TestingT.Helper-style hooks;
// today the WARN message includes the package + function name so a
// regression is easy to grep for.
func (p *Plugin) setState(s State) {
	old := State(p.state.Load())
	if old != s && !validStateTransition(old, s) {
		// Use the package logger if available; otherwise fall back to
		// the default. We deliberately don't take a *Manager reference
		// here because Plugin.setState is also called outside the
		// Manager's lifecycle hooks (e.g. by safeInit's panic recovery).
		slog.Default().Warn(
			"plugins: invalid state transition (applied anyway)",
			slog.String("plugin", p.descriptor.Name),
			slog.String("from", old.String()),
			slog.String("to", s.String()),
			slog.String("hint", fmt.Sprintf(
				"valid transitions from %s do not include %s — see Plugin.setState doc",
				old, s,
			)),
		)
	}
	p.state.Store(int32(s))
}

// validStateTransition reports whether transitioning from `from` to
// `to` is consistent with the lifecycle described in [Plugin.setState].
// Self-transitions (from == to) are handled by the caller — they are
// considered no-ops, not valid transitions, so the caller should
// short-circuit them before consulting this function.
func validStateTransition(from, to State) bool {
	switch from {
	case StateLoaded:
		// Initial state — anything except going back to Loaded is OK.
		return to == StateReady || to == StateFailed || to == StateUnloaded
	case StateReady:
		// Operational — can fail or be unloaded.
		return to == StateFailed || to == StateUnloaded
	case StateFailed:
		// Terminal-ish — only Unload is allowed (preserves Err).
		return to == StateUnloaded
	case StateUnloaded:
		// Final — nothing else is allowed.
		return false
	default:
		// Unknown source state: be permissive so a future iota
		// addition does not lock the manager.
		return true
	}
}

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
