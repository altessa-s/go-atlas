// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"context"
	"errors"
	"iter"
	"log/slog"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/core/io/files"
	"github.com/altessa-s/go-atlas/core/runtime/panics"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Manager provides a facade for loading, discovering and managing dynamic
// .so plugins. It is safe for concurrent use.
type Manager struct {
	mu      sync.RWMutex
	plugins map[string]*Plugin
	opts    *options
	logger  *slog.Logger
	closed  atomic.Bool

	// Sandbox state. The sandbox is applied at most once over the manager's
	// lifetime; the result is cached so subsequent Loads observe the same
	// outcome instead of attempting to re-apply (rlimits cannot be raised
	// after they are lowered, and PR_SET_NO_NEW_PRIVS is irreversible).
	//
	// sandboxErr is stored atomically because [Manager.CheckHealth] reads it
	// without going through [sync.Once.Do], which would otherwise miss the
	// happens-before edge that Once provides only to its callers.
	//
	// sandboxApply is the function that performs the actual primitives. It
	// defaults to the build-tagged [applySandbox] but tests in this package
	// override it with a stub. Per-Manager rather than package-global so
	// parallel tests cannot race on a shared hook.
	sandboxOnce  sync.Once
	sandboxErr   atomic.Pointer[errBox]
	sandboxApply func(SandboxOptions) error

	// Watcher state. Protected by watchMu so that iterator reads on mu
	// (taken by [Manager.Reload] from inside the watch goroutine) do not
	// contend with lifecycle transitions.
	watchMu   sync.Mutex
	watchCtx  context.Context
	watchStop context.CancelFunc
	watchDone chan struct{}
	watching  bool
}

// NewManager creates a new plugin Manager.
//
// Example:
//
//	mgr := plugins.NewManager(
//	    plugins.WithDir("./plugins"),
//	    plugins.WithDisabled("broken-plugin.so"),
//	)
func NewManager(opt ...Option) *Manager {
	opts := newOptions(opt...)
	return &Manager{
		plugins:      make(map[string]*Plugin),
		opts:         opts,
		logger:       opts.logger,
		sandboxApply: applySandbox,
	}
}

// Load scans the configured directory for .so files and loads them
// according to the Load/Disabled configuration.
//
// Plugins that fail to initialize are marked as [StateFailed] and recorded
// in the returned (joined) error; the remaining plugins continue loading.
//
// Concurrent calls to Load are safe but redundant: the second caller will
// race the first to register each plugin and the loser receives a wrapped
// [ErrPluginAlreadyLoaded] in its joined error. Production code should
// invoke Load exactly once at startup, typically through the factory.
func (m *Manager) Load(ctx context.Context) error {
	if m.closed.Load() {
		return ErrManagerClosed
	}

	if err := m.ensureSandbox(); err != nil {
		return err
	}

	pluginFiles, err := m.resolvePluginFiles()
	if err != nil {
		return err
	}

	if len(pluginFiles) == 0 {
		m.logger.Info("no plugins to load", slog.String("dir", m.opts.dir))
		return nil
	}

	m.logger.Info("loading plugins",
		slog.String("dir", m.opts.dir),
		slog.Int("count", len(pluginFiles)),
	)

	var loadErrors []error
	for _, file := range pluginFiles {
		if err := m.loadPlugin(ctx, file); err != nil {
			loadErrors = append(loadErrors, err)
		}
	}

	return errors.Join(loadErrors...)
}

// Reload scans the plugin directory and loads any new plugins that have
// not been loaded yet. Already loaded plugins (matched by source file name)
// are skipped, making the operation idempotent and safe to invoke from the
// filesystem watcher ([Manager.StartWatching]) after every debounced event.
//
// Modifications to files that are already mapped into the process are not
// reloaded: Go's plugin package cannot unload or replace code that has been
// opened. To upgrade an in-process plugin the host must be restarted.
func (m *Manager) Reload(ctx context.Context) error {
	if m.closed.Load() {
		return ErrManagerClosed
	}

	if err := m.ensureSandbox(); err != nil {
		return err
	}

	pluginFiles, err := m.resolvePluginFiles()
	if err != nil {
		return err
	}

	// Snapshot already-loaded source filenames. p.path is immutable after
	// construction so reading it under RLock is sufficient.
	m.mu.RLock()
	loadedPaths := make(map[string]struct{}, len(m.plugins))
	for _, p := range m.plugins {
		loadedPaths[filepath.Base(p.path)] = struct{}{}
	}
	m.mu.RUnlock()

	var loadErrors []error
	for _, file := range pluginFiles {
		if _, exists := loadedPaths[file]; exists {
			continue
		}
		if err := m.loadPlugin(ctx, file); err != nil {
			loadErrors = append(loadErrors, err)
		}
	}

	return errors.Join(loadErrors...)
}

// Get retrieves a loaded plugin by name.
// Returns [ErrPluginNotFound] if the plugin is not registered.
func (m *Manager) Get(name string) (*Plugin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, exists := m.plugins[name]
	if !exists {
		return nil, ErrPluginNotFound
	}
	return p, nil
}

// MustGet retrieves a loaded plugin by name or panics if not found.
//
// Use MustGet only at startup or in test setup where a missing plugin is a
// programmer error and there is no recovery path. In all other cases prefer
// [Manager.Get] and handle [ErrPluginNotFound] explicitly.
func (m *Manager) MustGet(name string) *Plugin {
	return panics.MustResult(m.Get(name))
}

// Plugins returns an iterator over all loaded plugins (name, plugin).
//
// The iteration order is unspecified — Go map iteration is randomized and
// the manager does not impose a stable ordering. Sort externally if a stable
// order is required.
//
// The iterator holds the manager's read lock for the duration of the range
// loop. The loop body must not call back into [Manager] methods that take
// the write lock (such as [Manager.Unload] or [Manager.Close]) — doing so
// would deadlock. To collect a snapshot for later mutation, copy the
// pointers into a local slice and break out of the loop first.
func (m *Manager) Plugins() iter.Seq2[string, *Plugin] {
	return func(yield func(string, *Plugin) bool) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		for name, p := range m.plugins {
			if !yield(name, p) {
				return
			}
		}
	}
}

// Names returns an iterator over the names of all loaded plugins.
//
// The iterator holds the manager's read lock for the duration of the range
// loop; see [Manager.Plugins] for the deadlock caveat.
func (m *Manager) Names() iter.Seq[string] {
	return func(yield func(string) bool) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		for name := range m.plugins {
			if !yield(name) {
				return
			}
		}
	}
}

// Ready returns an iterator over plugins that are in [StateReady].
//
// The iterator holds the manager's read lock for the duration of the range
// loop; see [Manager.Plugins] for the deadlock caveat.
func (m *Manager) Ready() iter.Seq2[string, *Plugin] {
	return func(yield func(string, *Plugin) bool) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		for name, p := range m.plugins {
			if p.State() != StateReady {
				continue
			}
			if !yield(name, p) {
				return
			}
		}
	}
}

// LookupAll finds all ready plugins that export a symbol with the given name.
// This is the primary mechanism for services to discover providers:
//
//	for p, sym := range mgr.LookupAll("AuthProvider") {
//	    provider, ok := sym.(auth.Provider)
//	    if !ok { continue }
//	    // use provider
//	}
//
// The iterator holds the manager's read lock for the duration of the range
// loop; see [Manager.Plugins] for the deadlock caveat.
func (m *Manager) LookupAll(symbol string) iter.Seq2[*Plugin, any] {
	return func(yield func(*Plugin, any) bool) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		for _, p := range m.plugins {
			if p.State() != StateReady {
				continue
			}
			if sym, ok := p.Lookup(symbol); ok {
				if !yield(p, sym) {
					return
				}
			}
		}
	}
}

// Unload removes a plugin from the manager and marks it as [StateUnloaded].
// Returns [ErrPluginNotFound] if no plugin is registered under the given name.
//
// Note: Go's plugin package does not support actually unloading .so files
// from the process; this only removes the plugin from the manager's registry.
//
// The method is named Unload (not Unregister) to mirror the Load/Unload verb
// pair: there is no public Register on this manager — registration happens
// implicitly during [Manager.Load] when scanning the configured directory.
// The lifecycle semantics (transitioning to [StateUnloaded]) also map more
// naturally to "unload" than to a generic "unregister".
func (m *Manager) Unload(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, exists := m.plugins[name]
	if !exists {
		return ErrPluginNotFound
	}

	p.setState(StateUnloaded)
	delete(m.plugins, name)

	m.logger.Info("plugin unloaded", slog.String("plugin", name))
	return nil
}

// Len returns the number of loaded plugins.
func (m *Manager) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.plugins)
}

// Close marks all plugins as unloaded and clears the manager. If a watcher
// is running it is stopped first and this method blocks until the watch
// goroutine has exited. Subsequent calls to [Manager.Load] return
// [ErrManagerClosed]. Close is idempotent.
func (m *Manager) Close() error {
	if m.closed.Swap(true) {
		return nil
	}

	// Stop the watcher before clearing state so any in-flight Reload from
	// the watch goroutine completes against the still-valid plugins map.
	m.StopWatching()

	m.mu.Lock()
	defer m.mu.Unlock()

	for name, p := range m.plugins {
		p.setState(StateUnloaded)
		m.logger.Debug("plugin closed", slog.String("plugin", name))
	}

	clear(m.plugins)
	return nil
}

// resolvePluginFiles lists .so files in the configured directory and applies
// load/disabled filtering. The returned slice contains base filenames only.
func (m *Manager) resolvePluginFiles() ([]string, error) {
	if !files.DirExists(m.opts.dir) {
		return nil, coreerrs.Wrapf(ErrDirNotFound, "directory %q", m.opts.dir)
	}

	var candidates []string
	for entry, err := range files.Walk(m.opts.dir,
		files.WithExtensions(".so"),
		files.WithFileTypes(files.FileTypeRegular),
	) {
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "walk plugin directory")
		}
		candidates = append(candidates, entry.Name())
	}

	// Load takes priority over Disabled when both are configured.
	if len(m.opts.load) > 0 {
		return keepListed(candidates, m.opts.load), nil
	}
	if len(m.opts.disabled) > 0 {
		return dropListed(candidates, m.opts.disabled), nil
	}
	return candidates, nil
}

// keepListed returns the elements of candidates that appear in the allow list.
func keepListed(candidates, allow []string) []string {
	allowed := setOf(allow)
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if _, ok := allowed[c]; ok {
			out = append(out, c)
		}
	}
	return out
}

// dropListed returns the elements of candidates that do not appear in the deny list.
func dropListed(candidates, deny []string) []string {
	denied := setOf(deny)
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if _, ok := denied[c]; !ok {
			out = append(out, c)
		}
	}
	return out
}

// setOf returns a set view of items as map[string]struct{}.
func setOf(items []string) map[string]struct{} {
	return coremaps.FromSliceWith(items, func(s string) (string, struct{}) {
		return s, struct{}{}
	})
}

// loadPlugin opens a single .so file, resolves its descriptor, and runs Init
// if present. Returned errors carry the failing plugin filename and wrap the
// relevant sentinel ([ErrNoDescriptor], [ErrInvalidDescriptor], etc.) for
// programmatic inspection via [errors.Is]. Errors are not logged here — the
// caller (typically [Manager.Load]) joins them so the application can decide
// how to surface failures.
func (m *Manager) loadPlugin(ctx context.Context, filename string) error {
	path := filepath.Join(m.opts.dir, filename)

	raw, err := openPlugin(path)
	if err != nil {
		return coreerrs.Wrapf(err, "plugin %q", filename)
	}

	lookup := symbolLookupFromPlugin(raw)
	desc, err := resolveDescriptor(lookup)
	if err != nil {
		return coreerrs.Wrapf(err, "plugin %q", filename)
	}

	p := &Plugin{
		descriptor: *desc,
		path:       path,
		raw:        raw,
		loadedAt:   time.Now(),
	}
	p.setState(StateLoaded)

	// Check for name collision and register.
	m.mu.Lock()
	if _, exists := m.plugins[desc.Name]; exists {
		m.mu.Unlock()
		return coreerrs.Wrapf(ErrPluginAlreadyLoaded, "plugin %q (file %q)", desc.Name, filename)
	}
	m.plugins[desc.Name] = p
	m.mu.Unlock()

	// Resolve and run Init if present. Both variable and function declaration
	// forms are accepted; a mis-typed Init symbol is logged but does not fail
	// the load — the plugin is still registered and enters StateReady.
	initFn, err := resolveInit(lookup)
	if err != nil {
		m.logger.Warn("plugin Init symbol has unsupported type; skipping",
			slog.String("plugin", desc.Name),
			slog.Any("error", err),
		)
	}
	if initFn != nil {
		// safeInit reports panics via the panic recovery handler (which logs
		// the stack); we propagate the error so the caller can decide.
		if initErr := m.safeInit(ctx, p, initFn); initErr != nil {
			return initErr
		}
	}

	if p.State() == StateLoaded {
		p.setState(StateReady)
	}

	m.logger.Info("plugin loaded",
		slog.String("plugin", desc.Name),
		slog.String("version", desc.Version),
		slog.String("file", filename),
		slog.String("state", p.State().String()),
	)

	return nil
}

// ensureSandbox applies the configured sandbox primitives exactly once and
// caches the result. Subsequent invocations return the same outcome instead
// of attempting to re-apply (most rlimits cannot be raised after they are
// lowered, and PR_SET_NO_NEW_PRIVS is irreversible).
//
// When the sandbox is disabled this is a cheap no-op. When applying fails on
// the first call, the cached error is propagated to every future Load and
// Reload — the manager refuses to load plugins under a half-applied sandbox.
//
// The actual syscalls are performed by [Manager.sandboxApply], which defaults
// to the build-tagged [applySandbox] and is overridden by tests with a stub.
// Before calling the apply hook, [expandSandboxOptions] merges in any
// auto-added Landlock paths (plugin directory, system library directories).
func (m *Manager) ensureSandbox() error {
	m.sandboxOnce.Do(func() {
		if !m.opts.sandbox.Enabled {
			return
		}
		effective := m.expandSandboxOptions(m.opts.sandbox)
		if err := m.sandboxApply(effective); err != nil {
			m.sandboxErr.Store(&errBox{err: err})
			m.logger.Error("plugin sandbox setup failed", slog.Any("error", err))
			return
		}
		m.logger.Info("plugin sandbox active",
			slog.Bool("noNewPrivs", effective.NoNewPrivs),
			slog.Int64("memoryLimitBytes", effective.MemoryLimitBytes),
			slog.Int64("maxOpenFiles", effective.MaxOpenFiles),
			slog.Int64("maxProcesses", effective.MaxProcesses),
			slog.Int64("maxFileSizeBytes", effective.MaxFileSizeBytes),
			slog.Bool("disableCoreDumps", effective.DisableCoreDumps),
			slog.Bool("landlock", effective.Landlock.Enabled),
			slog.Int("landlockReadPaths", len(effective.Landlock.ReadPaths)),
			slog.Int("landlockReadWritePaths", len(effective.Landlock.ReadWritePaths)),
		)
	})
	return m.sandboxFailure()
}

// defaultLandlockSystemLibPaths lists the filesystem paths that glibc and
// musl dynamic loaders typically require. When [LandlockOptions.AllowSystemLibs]
// is true, these are merged into the ReadPaths allowlist.
//
// The list intentionally stays minimal — covering RHEL, Debian, Ubuntu,
// Fedora, Alpine, and most other mainstream distros. It is NOT suitable for
// NixOS (libs live under /nix/store), chroot jails, or deployments with
// custom library prefixes — those should use explicit ReadPaths and leave
// AllowSystemLibs off.
var defaultLandlockSystemLibPaths = []string{
	"/lib",
	"/lib64",
	"/usr/lib",
	"/usr/lib64",
}

// expandSandboxOptions returns a copy of o with any auto-added Landlock
// paths (plugin directory, system library directories) merged into
// ReadPaths. The ReadPaths slice is cloned before the merge so the caller's
// original slice is never mutated.
//
// When Landlock is disabled or no auto-add flag is set, the returned value
// is the input unchanged (safe because Go passes the struct by value and
// we only mutate the cloned slice, not the caller's).
func (m *Manager) expandSandboxOptions(o SandboxOptions) SandboxOptions {
	if !o.Landlock.Enabled {
		return o
	}

	var extra []string
	if o.Landlock.AllowPluginDir && m.opts.dir != "" {
		extra = append(extra, m.opts.dir)
	}
	if o.Landlock.AllowSystemLibs {
		extra = append(extra, defaultLandlockSystemLibPaths...)
	}
	if len(extra) == 0 {
		return o
	}

	// slices.Concat allocates a fresh backing array so the caller's
	// ReadPaths slice is never mutated, regardless of its capacity.
	o.Landlock.ReadPaths = slices.Concat(o.Landlock.ReadPaths, extra)
	return o
}

// sandboxFailure returns the cached sandbox setup error, or nil if the
// sandbox was disabled, has not been attempted yet, or applied successfully.
// It is safe for concurrent use because [sandboxErr] is an [atomic.Pointer].
func (m *Manager) sandboxFailure() error {
	if box := m.sandboxErr.Load(); box != nil {
		return box.err
	}
	return nil
}

// safeInit calls the plugin's Init function with panic recovery and a timeout
// derived from the manager options. Panics are recorded on the plugin and
// returned as wrapped [ErrPluginPanicked]; init errors are wrapped with
// [ErrPluginFailed]. The plugin's state is updated atomically.
//
// The named return value retErr exists so the deferred panic handler can
// communicate the recovered error back to the caller.
func (m *Manager) safeInit(ctx context.Context, p *Plugin, initFn func(context.Context) error) (retErr error) {
	defer panics.HandleWithOpts(ctx,
		panics.NewHandleOpts().SetReallyPanic(false),
		func(_ context.Context, r any) {
			panicErr := coreerrs.Wrapf(ErrPluginPanicked, "plugin %q panicked during init: %v", p.Name(), r)
			p.setErr(panicErr)
			p.setState(StateFailed)
			retErr = panicErr
		},
	)

	initCtx, cancel := context.WithTimeout(ctx, m.opts.initTimeout)
	defer cancel()

	if err := initFn(initCtx); err != nil {
		failErr := coreerrs.Wrapf(ErrPluginFailed, "plugin %q: %v", p.Name(), err)
		p.setErr(failErr)
		p.setState(StateFailed)
		return failErr
	}

	p.setState(StateReady)
	return nil
}
