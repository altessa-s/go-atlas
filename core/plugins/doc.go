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
//   - Landlock filesystem allowlisting (Linux 5.13+)
//
// All primitives are configured via [config.PluginsSandbox] and applied
// lazily on the first [Manager.Load]. The Landlock pass runs after the
// rlimits because landlock_restrict_self requires PR_SET_NO_NEW_PRIVS to
// have been set first (or CAP_SYS_ADMIN, which production hosts should
// not have).
//
// Honest limitations:
//
//   - Linux only. On other platforms an enabled sandbox makes [Manager.Load]
//     fail with [ErrSandboxUnsupported]. Landlock additionally requires
//     kernel 5.13 or newer.
//   - Applied lazily and irreversible. Host code that runs before the first
//     Load is unrestricted.
//   - Process-wide. Every limit applies to the entire host, not just plugin
//     code. Setting MaxOpenFiles too low will starve a database connection
//     pool the host owns; forgetting to allowlist /lib64 in the Landlock
//     ReadPaths will break dlopen for every plugin.
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
// seccomp-BPF is not part of this version. It is tracked as a future
// extension.
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
