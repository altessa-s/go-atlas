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

	// sandboxSystemLibPaths is the list of system library directories
	// merged into the Landlock allowlist when [LandlockOptions.AllowSystemLibs]
	// is true. Defaults to [defaultLandlockSystemLibPaths]; tests in this
	// package override it to point at temp directories so the
	// expandSandboxOptions test suite is independent of the host's actual
	// filesystem layout (per-Manager rather than package-global so parallel
	// tests cannot race on a shared override).
	sandboxSystemLibPaths []string

	// reloadErr stores the most recent error returned by [Manager.Reload]
	// when invoked from the filesystem watcher goroutine. The watcher is
	// best-effort and does not propagate Reload errors through the call
	// chain (its only caller is itself), so without this field a stuck
	// watcher would be invisible to operators relying on health checks.
	// CheckHealth downgrades to StatusDegraded when this is non-nil and
	// the rest of the manager is otherwise healthy. Stored as
	// atomic.Pointer so [Manager.CheckHealth] can read without holding
	// any lock.
	reloadErr atomic.Pointer[errBox]

	// watcherReloadHook is a test-only hook fired after every
	// watcher-driven Reload attempt. The argument is the (possibly nil)
	// error returned by Reload. Production code leaves it nil; tests in
	// this package set it to drive deterministic synchronization
	// instead of polling Manager.Len() or sleeping for an arbitrary
	// duration. Per-Manager so parallel tests cannot race on a global.
	watcherReloadHook atomic.Pointer[func(error)]

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
		plugins:               make(map[string]*Plugin),
		opts:                  opts,
		logger:                opts.logger,
		sandboxApply:          applySandbox,
		sandboxSystemLibPaths: defaultLandlockSystemLibPaths,
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

// pluginEntry is a (name, plugin) pair used by snapshot helpers below.
// Defined as a struct rather than a 2-tuple so the snapshot allocators
// can return a single slice without losing the name <-> *Plugin pairing.
type pluginEntry struct {
	name string
	p    *Plugin
}

// snapshotPlugins returns a stable snapshot of the plugin registry.
// The slice is owned by the caller and never aliases the manager's
// internal map. Order is unspecified (map iteration is randomized).
//
// Snapshotting under the read lock and then iterating outside it is
// the safe pattern: it lets the caller's loop body call back into any
// Manager method (including write-lock methods like [Manager.Unload]
// or [Manager.Close]) without risk of deadlock. The trade-off is that
// the snapshot may be stale by the time the caller observes it — a
// plugin that was unloaded mid-iteration may still be yielded. For the
// SPI discovery use case (look up symbols and dispatch) this is the
// right trade: stale data is fine; deadlocks are not.
func (m *Manager) snapshotPlugins() []pluginEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]pluginEntry, 0, len(m.plugins))
	for name, p := range m.plugins {
		out = append(out, pluginEntry{name: name, p: p})
	}
	return out
}

// Plugins returns an iterator over all loaded plugins (name, plugin).
//
// The iteration order is unspecified — Go map iteration is randomized and
// the manager does not impose a stable ordering. Sort externally if a stable
// order is required.
//
// The iterator captures a snapshot of the registry under the manager's
// read lock and then yields from that snapshot WITHOUT holding any lock,
// so the loop body is free to call any [Manager] method — including
// write-lock methods like [Manager.Unload] and [Manager.Close] — without
// deadlock risk. The snapshot may be slightly stale by the time the
// caller observes it (a plugin unloaded mid-iteration is still yielded);
// callers that need strict consistency should re-check [Plugin.State]
// inside the loop body.
func (m *Manager) Plugins() iter.Seq2[string, *Plugin] {
	return func(yield func(string, *Plugin) bool) {
		for _, e := range m.snapshotPlugins() {
			if !yield(e.name, e.p) {
				return
			}
		}
	}
}

// Names returns an iterator over the names of all loaded plugins.
//
// Like [Manager.Plugins], the iterator yields from a snapshot taken under
// the read lock; the loop body may safely call any Manager method.
func (m *Manager) Names() iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, e := range m.snapshotPlugins() {
			if !yield(e.name) {
				return
			}
		}
	}
}

// Ready returns an iterator over plugins that are in [StateReady].
//
// Like [Manager.Plugins], the iterator yields from a snapshot taken under
// the read lock; the loop body may safely call any Manager method. The
// state check is performed against [Plugin.State] at yield time, so a
// plugin that transitioned out of [StateReady] between the snapshot and
// the yield is correctly excluded.
func (m *Manager) Ready() iter.Seq2[string, *Plugin] {
	return func(yield func(string, *Plugin) bool) {
		for _, e := range m.snapshotPlugins() {
			if e.p.State() != StateReady {
				continue
			}
			if !yield(e.name, e.p) {
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
// Like [Manager.Plugins], the iterator yields from a snapshot taken under
// the read lock; the loop body may safely call any Manager method.
func (m *Manager) LookupAll(symbol string) iter.Seq2[*Plugin, any] {
	return func(yield func(*Plugin, any) bool) {
		for _, e := range m.snapshotPlugins() {
			if e.p.State() != StateReady {
				continue
			}
			if sym, ok := e.p.Lookup(symbol); ok {
				if !yield(e.p, sym) {
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

// loadPlugin opens a single .so file, resolves its descriptor, runs Init
// if present, and only then registers the plugin in the manager's
// registry. Returned errors carry the failing plugin filename and wrap
// the relevant sentinel ([ErrNoDescriptor], [ErrInvalidDescriptor], etc.)
// for programmatic inspection via [errors.Is]. Errors are not logged here
// — the caller (typically [Manager.Load]) joins them so the application
// can decide how to surface failures.
//
// Registration sequencing: the plugin enters the registry only AFTER
// safeInit returns and the plugin has been transitioned to either
// StateReady or StateFailed. Other goroutines calling
// [Manager.Get] / [Manager.Plugins] / [Manager.LookupAll] therefore
// never observe a half-initialized plugin in StateLoaded with Init still
// running. The trade-off is that name collisions are detected slightly
// later — after the new plugin's Init has executed — but plugin Init
// is supposed to be idempotent and side-effect-free for the host
// process address space, so a redundant Init on a name collision is
// harmless. The collision check is still atomic with respect to
// concurrent loaders thanks to the write lock around the registry
// transaction.
//
// A present-but-mis-typed Init symbol is treated as a load failure
// (see [ErrInvalidInit] in [resolveInit]) and the plugin is NOT
// registered.
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

	// Pre-flight name collision check. This is a hint, not a contract:
	// a concurrent loader could register the same name between this
	// read and the final commit below. The post-Init registration
	// transaction below repeats the check under the write lock and is
	// the authoritative one. We do this hint here so that an obviously
	// duplicate plugin (e.g. operator dropped a renamed copy of an
	// already-loaded .so) does not waste an Init call before failing.
	m.mu.RLock()
	_, exists := m.plugins[desc.Name]
	m.mu.RUnlock()
	if exists {
		return coreerrs.Wrapf(ErrPluginAlreadyLoaded, "plugin %q (file %q)", desc.Name, filename)
	}

	p := newPlugin(*desc, path, raw)

	// Resolve Init outside the lock — both forms are accepted; see
	// [resolveInit] for the supported shapes. A present-but-mis-typed
	// Init is a load failure: it almost always indicates a host/plugin
	// version skew (signature changed without rebuilding the plugin)
	// and the plugin should be quarantined rather than promoted to
	// StateReady with its initialization silently skipped.
	initFn, err := resolveInit(lookup)
	if err != nil {
		failErr := coreerrs.Wrapf(err, "plugin %q", desc.Name)
		p.setErr(failErr)
		p.setState(StateFailed)
		m.logger.Error("plugin Init symbol has unsupported type",
			slog.String("plugin", desc.Name),
			slog.Any("error", err),
		)
		return failErr
	}
	if initFn != nil {
		// safeInit reports panics via the panic recovery handler
		// (which logs the stack); we propagate the error so the
		// caller can decide. The plugin is in StateFailed by the
		// time safeInit returns an error, so the caller sees a
		// fully-classified failure.
		if initErr := m.safeInit(ctx, p, initFn); initErr != nil {
			return initErr
		}
	}
	if p.State() == StateLoaded {
		p.setState(StateReady)
	}

	// Register only after Init has run to completion. This is the
	// authoritative collision check: it runs under the write lock so
	// two concurrent loaders racing on the same plugin name are
	// guaranteed to see exactly one winner.
	m.mu.Lock()
	if _, exists := m.plugins[desc.Name]; exists {
		m.mu.Unlock()
		// The plugin we just initialized lost the race. Mark it
		// as unloaded so [Plugin.State] reflects reality if some
		// other code path holds a reference to it. The other
		// loader's plugin remains the canonical one.
		p.setState(StateUnloaded)
		return coreerrs.Wrapf(ErrPluginAlreadyLoaded, "plugin %q (file %q)", desc.Name, filename)
	}
	m.plugins[desc.Name] = p
	m.mu.Unlock()

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
		// Fail-fast on operator misconfiguration so a malformed
		// sandbox surfaces as ErrSandboxFailed wrapping the
		// validation errors instead of leaking the underlying
		// kernel errno.
		if err := m.opts.sandbox.Validate(); err != nil {
			wrapped := coreerrs.JoinWrap(ErrSandboxFailed, err)
			m.sandboxErr.Store(&errBox{err: wrapped})
			m.logger.Error("plugin sandbox configuration is invalid",
				slog.Any("error", err))
			return
		}
		effective := m.expandSandboxOptions(m.opts.sandbox)
		m.warnPerThreadPrimitives(effective)
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

// warnPerThreadPrimitives logs a one-shot WARN when the operator enabled
// sandbox primitives that the kernel applies per-thread (capabilities,
// Landlock, NO_NEW_PRIVS) rather than per-process. The Go runtime has
// already created multiple OS threads (sysmon, GC, netpoll, GOMAXPROCS
// workers) by the time the manager runs ensureSandbox, and Go does not
// expose any way to iterate or pin every existing thread. The kernel
// syscalls land on the goroutine's current OS thread; peer threads keep
// their original capability set / Landlock domain / NNP bit.
//
// In practice this means: a malicious goroutine that gets scheduled onto
// a peer thread can still call dropped-cap syscalls or read paths
// outside the Landlock allowlist. Real process-wide isolation requires
// dropping these privileges externally — a systemd unit with
// CapabilityBoundingSet=, NoNewPrivileges=yes, and a Linux container
// runtime with --cap-drop, or a small C launcher that drops privileges
// before execve(2) of the Go binary.
//
// rlimits and seccomp+TSYNC are NOT in this warning because the kernel
// gives them real process-wide semantics: setrlimit(2) is per-process,
// and seccomp(SET_MODE_FILTER, TSYNC) propagates the installed filter
// to every peer thread atomically.
//
// See docs/plugins.md "Recommended deployment" for the full sysadmin
// runbook.
func (m *Manager) warnPerThreadPrimitives(o SandboxOptions) {
	var perThread []string
	if o.NoNewPrivs {
		perThread = append(perThread, "noNewPrivs")
	}
	if o.Capabilities.Enabled {
		perThread = append(perThread, "capabilities")
	}
	if o.Landlock.Enabled {
		perThread = append(perThread, "landlock")
	}
	if len(perThread) == 0 {
		return
	}
	m.logger.Warn(
		"plugin sandbox uses per-thread Linux primitives that are best-effort in Go; "+
			"the Go runtime has multiple OS threads by the time the sandbox runs and "+
			"these primitives only restrict the calling thread. For real process-wide "+
			"isolation, drop privileges externally via systemd "+
			"(CapabilityBoundingSet=, NoNewPrivileges=yes), a container runtime "+
			"(--cap-drop, --security-opt=no-new-privileges), or a C launcher before "+
			"execve. See docs/plugins.md#recommended-deployment.",
		slog.Any("primitives", perThread),
	)
}

// defaultLandlockSystemLibPaths lists the filesystem paths that glibc and
// musl dynamic loaders typically require. When [LandlockOptions.AllowSystemLibs]
// is true, these are filtered through [files.DirExists] and the survivors
// are merged into the ReadPaths allowlist.
//
// The list intentionally stays minimal — covering RHEL, Debian, Ubuntu,
// Fedora, Alpine, and most other mainstream distros. It is NOT suitable for
// NixOS (libs live under /nix/store), chroot jails, or deployments with
// custom library prefixes — those should use explicit ReadPaths and leave
// AllowSystemLibs off. The DirExists filter exists because Landlock's
// add_rule fails the entire ruleset setup with ENOENT if any listed path
// is missing — without filtering, a host that lacks /lib64 (Alpine on
// some configs, Debian without multilib, NixOS) would fail Manager.Load
// even when the operator just wanted "best-effort common system libs".
var defaultLandlockSystemLibPaths = []string{
	"/lib",
	"/lib64",
	"/usr/lib",
	"/usr/lib64",
}

// expandSandboxOptions returns a copy of o with any auto-added Landlock
// paths (plugin directory, system library directories) merged into
// ReadPaths. The ReadPaths slice is cloned before the merge so the caller's
// original slice is never mutated. Duplicate entries (after the merge)
// are removed via [slices.Compact] on a sorted-equality basis: an
// operator who lists `/lib64` explicitly and also enables
// `AllowSystemLibs` should not see `/lib64` registered twice in the
// Landlock allowlist (the kernel tolerates the duplicate but it
// pollutes operator-facing logs).
//
// When Landlock is disabled or no auto-add flag is set, the returned value
// is the input unchanged (safe because Go passes the struct by value and
// we only mutate the cloned slice, not the caller's).
//
// AllowSystemLibs paths are filtered through [files.DirExists] before
// being added: missing directories are silently dropped rather than
// surfaced as a Landlock setup failure. The plugin manager logs the
// effective list at Info level so operators can audit what actually
// got allowlisted on this host.
func (m *Manager) expandSandboxOptions(o SandboxOptions) SandboxOptions {
	if !o.Landlock.Enabled {
		return o
	}

	var extra []string
	if o.Landlock.AllowPluginDir && m.opts.dir != "" {
		extra = append(extra, m.opts.dir)
	}
	if o.Landlock.AllowSystemLibs {
		var skipped []string
		for _, p := range m.sandboxSystemLibPaths {
			if files.DirExists(p) {
				extra = append(extra, p)
				continue
			}
			skipped = append(skipped, p)
		}
		if len(skipped) > 0 {
			m.logger.Info(
				"plugin sandbox: skipping missing system library paths "+
					"(AllowSystemLibs is best-effort; explicit ReadPaths are not filtered)",
				slog.Any("skipped", skipped),
			)
		}
	}
	if len(extra) == 0 {
		return o
	}

	// slices.Concat allocates a fresh backing array so the caller's
	// ReadPaths slice is never mutated, regardless of its capacity.
	merged := slices.Concat(o.Landlock.ReadPaths, extra)
	o.Landlock.ReadPaths = dedupePreserveOrder(merged)
	return o
}

// dedupePreserveOrder removes duplicate strings from s while keeping
// the first occurrence's position. Used by expandSandboxOptions to
// suppress redundant entries when an operator lists a system lib path
// explicitly AND enables AllowSystemLibs.
//
// O(n) using a small set; not worth the cognitive overhead of an
// in-place algorithm because the slices are <20 entries in practice.
func dedupePreserveOrder(s []string) []string {
	if len(s) < 2 {
		return s
	}
	seen := make(map[string]struct{}, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
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

// LastWatcherReloadErr returns the most recent error from a
// watcher-driven [Manager.Reload], or nil when the last reload
// succeeded (or the watcher has not run). The watcher is a
// best-effort background task: a failed reload is logged but does not
// stop the watcher or fail other plugins, and without this accessor a
// stuck watcher would be invisible to operators.
//
// [Manager.CheckHealth] reads this value and downgrades to
// [health.StatusDegraded] when it is non-nil and the rest of the
// manager is otherwise healthy. Hosts that wire reload failures into
// metrics or alerts can poll this method directly.
//
// The error is cleared back to nil after the next successful reload.
func (m *Manager) LastWatcherReloadErr() error {
	if box := m.reloadErr.Load(); box != nil {
		return box.err
	}
	return nil
}

// recordWatcherReloadResult stores or clears the cached watcher reload
// error after each debounce-driven [Manager.Reload] and fires the
// test-only [watcherReloadHook] if one is installed. Called from the
// watch goroutine only.
func (m *Manager) recordWatcherReloadResult(err error) {
	if err == nil {
		m.reloadErr.Store(nil)
	} else {
		m.reloadErr.Store(&errBox{err: err})
	}
	if hookPtr := m.watcherReloadHook.Load(); hookPtr != nil && *hookPtr != nil {
		(*hookPtr)(err)
	}
}

// safeInit calls the plugin's Init function with panic recovery and a timeout
// derived from the manager options. Panics are recorded on the plugin and
// returned as wrapped [ErrPluginPanicked]; init errors are wrapped with
// [ErrPluginFailed]. The plugin's state is updated atomically.
//
// The named return value retErr exists so the deferred panic handler can
// communicate the recovered error back to the caller. If retErr is already
// non-nil when the panic handler runs (Init returned an error and then a
// deferred plugin-side cleanup panicked), both errors are joined via
// [errors.Join] so neither signal is lost — losing the original init error
// would mask the root cause.
func (m *Manager) safeInit(ctx context.Context, p *Plugin, initFn func(context.Context) error) (retErr error) {
	defer panics.HandleWithOpts(ctx,
		panics.NewHandleOpts().SetReallyPanic(false),
		func(_ context.Context, r any) {
			panicErr := coreerrs.Wrapf(ErrPluginPanicked, "plugin %q panicked during init: %v", p.Name(), r)
			p.setErr(panicErr)
			p.setState(StateFailed)
			// Preserve any pre-existing retErr (e.g. an init
			// error that ran to completion before a deferred
			// cleanup inside the plugin panicked). Joining
			// keeps the original cause discoverable via
			// errors.Is alongside ErrPluginPanicked.
			if retErr != nil {
				retErr = errors.Join(retErr, panicErr)
			} else {
				retErr = panicErr
			}
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
