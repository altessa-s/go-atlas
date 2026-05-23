// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package plugins provides a dynamic plugin manager for loading and managing
// .so plugins at runtime, inspired by Keycloak's SPI (Service Provider Interface).
//
// The package is framework-agnostic: it manages plugin lifecycle (loading,
// initialization, crash protection) while consuming services define their own
// provider interfaces and discover them via symbol lookup.
//
// # Architecture
//
// The package is built around three main components:
//   - [Manager]: Loads, discovers, and manages plugin lifecycle
//   - [Plugin]: Represents a loaded .so file with metadata and symbol lookup
//   - [Descriptor]: Metadata that every plugin must export
//
// # Plugin Contract
//
// Each .so plugin must export a package-level variable named Descriptor.
// Both the value form and the pointer form are accepted:
//
//	// Value form (recommended):
//	var Descriptor = plugins.Descriptor{
//	    Name:    "my-plugin",
//	    Version: "1.0.0",
//	}
//
//	// Pointer form (also supported):
//	var Descriptor = &plugins.Descriptor{
//	    Name:    "my-plugin",
//	    Version: "1.0.0",
//	}
//
// Optionally, a plugin may export an Init callback for one-shot setup work.
// Both idiomatic declarations are accepted:
//
//	// Function declaration (natural Go style):
//	func Init(ctx context.Context) error {
//	    // setup code
//	    return nil
//	}
//
//	// Variable form:
//	var Init = func(ctx context.Context) error {
//	    // setup code
//	    return nil
//	}
//
// Init is invoked under a timeout ([config.Plugins.InitTimeout]) and wrapped
// in panic recovery; failures mark the plugin as [StateFailed] and surface
// through the joined error returned by [Manager.Load]. Plugins without an
// Init symbol transition directly to [StateReady] after registration.
//
// A present-but-mis-typed Init symbol (wrong arity, wrong context type,
// returns no error) is treated as a load failure and wrapped in
// [ErrInvalidInit]. This is almost always a host/plugin version skew —
// the plugin must be rebuilt against the current host. The plugin is
// left in [StateFailed] so it is excluded from [Manager.Ready] /
// [Manager.LookupAll] iteration.
//
// # ABI and version-skew detection
//
// Go's [plugin.Open] requires the host and the plugin to be compiled with
// the same Go toolchain version and identical shared-dependency versions.
// Violations produce cryptic runtime errors. The package provides three
// mechanisms for early detection:
//
//   - [Descriptor.GoVersion] (advisory): set it to [runtime.Version] in
//     the plugin. The manager compares it against the host's own version
//     at load time and logs a warning on mismatch.
//   - DepInfo symbol (advisory): export a package-level variable named
//     DepInfo of type [*DepInfo] (use [NewDepInfoFromBuild] for
//     convenience). The manager compares the plugin's module dependency
//     graph against the host's [debug.ReadBuildInfo] and logs mismatched
//     shared modules.
//   - [Descriptor.HostVersion] (enforced): set it to the host service
//     semver (e.g. "2.1.0"). When the manager is configured via
//     [WithHostVersion] (the [factory] package wires this from
//     [appinfo.Version] automatically) the loader compares the major
//     components and rejects mismatches with [ErrHostVersionMismatch].
//     The strictness is controlled by [HostVersionMode]
//     (Enforce/Warn/Disabled). This is the recommended mechanism for
//     production-grade SemVer compatibility — the other two only surface
//     warnings.
//
// The first two checks are advisory because [plugin.Open] enforces the
// real Go-toolchain ABI; HostVersion is independent of that and reflects
// the host service's own SemVer compatibility contract.
//
// # Signature verification
//
// When enabled via [WithSignature], the manager verifies a detached
// cryptographic signature (.so.sig file) before [plugin.Open] executes
// any code. This prevents loading tampered or unauthorized .so files.
//
// Supported algorithms: Ed25519 (recommended), ECDSA P-256, RSA-PSS.
// The public key is configured once for the manager; each .so file
// has a companion .so.sig file containing the raw signature bytes.
//
// Three modes control behavior:
//   - [SignatureRequire] / [SignatureEnforce] (default): reject if .sig is missing
//     or invalid. This is the secure default for production.
//   - [SignatureWarn]: log a warning for missing .sig, but allow unsigned
//     plugins. Invalid signatures are always rejected.
//   - [SignatureDisabled]: no verification. SECURITY WARNING: Use only in
//     development or when explicitly required.
//
// Plugins that fail signature verification are quarantined automatically.
//
// # Quarantine
//
// When a plugin fails to load (broken .so, missing descriptor, Init error
// or panic), the manager records the file's SHA256 hash in an in-memory
// quarantine store. Subsequent [Manager.Load] and [Manager.Reload] calls
// skip quarantined files until the hash changes (operator deployed a fix).
// The [filesystem watcher] clears quarantine automatically when a WRITE
// event changes the file content.
//
// Services can also quarantine a running plugin explicitly via
// [Manager.Quarantine] when they observe runtime failures. The plugin
// is removed from the registry, marked [StateFailed], and its file hash
// is recorded. Use [Manager.Quarantined] to inspect the current
// quarantine store.
//
// # SPI version negotiation
//
// For versioned provider contracts, plugins export a companion
// version symbol alongside each provider symbol using the naming
// convention "<Symbol>SPIVersion" (e.g. AuthProviderSPIVersion of type
// [SPIVersion]). Hosts use [NegotiateAll] instead of [Manager.LookupAll]
// to filter providers by an [SPIConstraint]. Unversioned plugins are
// included unchanged for backwards compatibility.
//
// Any additional symbols can be exported for service-specific discovery via
// [Plugin.Lookup] or [Manager.LookupAll].
//
// # SPI Discovery
//
// Services discover providers by looking up a well-known symbol name across
// all ready plugins:
//
//	for p, sym := range mgr.LookupAll("AuthProvider") {
//	    provider, ok := sym.(*MyAuthProvider)
//	    if !ok {
//	        continue
//	    }
//	    // use provider
//	}
//
// # Crash Protection
//
// Plugin initialization is wrapped in panic recovery. If a plugin panics
// during Init, it is marked as [StateFailed] and the remaining plugins
// continue loading. Failed plugins are excluded from [Manager.LookupAll]
// and [Manager.Ready] iteration.
//
// # Security
//
// Loading a plugin executes arbitrary native code in the host process. Go's
// [plugin.Open] runs the plugin's package-level init functions BEFORE the
// manager has a chance to inspect the descriptor, so even rejecting a plugin
// for [ErrInvalidDescriptor] does not prevent its init code from running.
// There is no sandbox: a malicious plugin can read process memory, open
// files, perform network I/O, or call exit.
//
// Operate on the assumption that any .so file under the configured directory
// is fully trusted code shipped by the same supply chain as the host binary.
// Concretely:
//
//   - Restrict the plugin directory to a path writable only by the deployer
//     (not the application user).
//   - Do not load plugins from user uploads, network shares, or world-writable
//     locations.
//   - Pin plugin versions and verify integrity (checksum, signature) outside
//     the manager when the deployment surface allows it.
//
// # Sandbox mode
//
// The package can apply a small set of Linux process-hardening primitives
// before opening any .so file:
//
//   - PR_SET_NO_NEW_PRIVS
//   - RLIMIT_AS, RLIMIT_NOFILE, RLIMIT_NPROC, RLIMIT_FSIZE, RLIMIT_CORE
//   - Linux capability dropping (capset(2) / PR_CAPBSET_DROP /
//     PR_CAP_AMBIENT_*)
//   - Landlock filesystem allowlisting (Linux 5.13+)
//
// All primitives are configured via [config.PluginsSandbox] and applied
// lazily on the first [Manager.Load]. The order is fixed:
// noNewPrivs → rlimits → capabilities → landlock. Landlock runs last
// because landlock_restrict_self requires PR_SET_NO_NEW_PRIVS (or
// CAP_SYS_ADMIN, which production hosts should not have); the manager's
// own ordering also ensures that whatever the operator configured runs
// before any plugin code is loaded into the process address space.
//
// Honest limitations:
//
//   - Linux only. On other platforms an enabled sandbox makes [Manager.Load]
//     fail with [ErrSandboxUnsupported]. Landlock additionally requires
//     kernel 5.13 or newer.
//   - Applied lazily and irreversible. Host code that runs before the first
//     Load is unrestricted.
//   - rlimits are real process-wide. setrlimit(2) applies to the entire
//     host process, so MaxOpenFiles too low will starve the host's
//     database/HTTP/Redis pools — these are not plugin-scoped limits.
//   - NoNewPrivs, capabilities, and Landlock are per-thread in pure Go.
//     PR_SET_NO_NEW_PRIVS, capset(2), and landlock_restrict_self(2) are
//     all per-thread/per-task on Linux, and Go does not expose any way to
//     iterate or pin every existing OS thread. The Go runtime has multiple
//     threads by the time the manager runs ensureSandbox (sysmon, GC,
//     netpoll, GOMAXPROCS workers), so the sandbox only restricts the
//     goroutine's current thread; peer threads keep their original
//     capability set, NNP state, and Landlock domain. The manager logs a
//     WARN when any of these primitives is enabled to remind operators
//     that real process-wide isolation requires dropping privileges
//     externally — via systemd (CapabilityBoundingSet=, NoNewPrivileges=
//     yes), a Linux container runtime (--cap-drop,
//     --security-opt=no-new-privileges), or a small C launcher that drops
//     privileges before execve(2) of the Go binary. See docs/plugins.md
//     "Recommended deployment" for the runbook.
//   - Landlock is a strict allowlist by default. Operators must list the
//     plugin directory, the dynamic loader, libc, every shared library the
//     plugin imports, and every host-side path the application needs. Two
//     convenience flags merge common path sets at apply time:
//     [config.PluginsLandlock.AllowPluginDir] auto-adds the plugin
//     directory, and [config.PluginsLandlock.AllowSystemLibs] auto-adds
//     /lib, /lib64, /usr/lib, /usr/lib64 (glibc + musl mainstream distros).
//   - Defense in depth, not isolation. A malicious plugin still has full
//     access to host memory and can corrupt or exfiltrate anything in the
//     address space — see the Security section above. The sandbox reduces
//     blast radius for buggy plugins; it is not a security boundary.
//
// seccomp-BPF is available as a peer primitive in
// [github.com/altessa-s/go-atlas/core/runtime/seccomp] but is not wired
// into the plugin sandbox: the package installs a fixed denylist of
// dangerous syscalls process-wide via TSYNC, and consumers can call
// [seccomp.BlockDangerousSyscalls] directly during host startup before
// constructing the plugin manager. It is intentionally NOT operator-
// configurable from PluginsSandbox to keep the audited denylist
// stable.
//
// # Configuration
//
// The manager supports two filtering modes via [config.Plugins]:
//   - Load: explicit allowlist of plugin filenames (takes priority)
//   - Disabled: exclusion list; all plugins except these are loaded
//
// # Runtime watching
//
// When [config.Plugins.Watch] is true the factory starts a filesystem
// watcher ([Manager.StartWatching]) after the initial [Manager.Load].
// The watcher reacts to .so files appearing in the plugin directory and
// invokes [Manager.Reload] after a configurable debounce window.
//
// Only additions are acted upon. Modifications and removals are ignored
// because Go's plugin package cannot unload or replace code that is
// already mapped into the process — to upgrade a plugin the host service
// must be restarted. The watcher is a best-effort background task: a
// failed reload is logged but does not stop the watcher or fail other
// plugins.
//
// The watcher's lifetime is bound to the context passed to StartWatching;
// pass the application's long-lived context, not an initialization-only
// one. [Manager.Close] stops the watcher automatically.
//
// # Thread Safety
//
// All public methods of [Manager] are safe for concurrent use. See
// [Manager.Plugins] for the lock-holding caveat that applies to iterator
// methods.
package plugins
