# Plugins

```go
import "github.com/altessa-s/go-atlas/core/plugins"
```

The `plugins` package manages native Go `.so` plugins at runtime, following the SPI (Service Provider Interface) pattern from Keycloak. It handles only
lifecycle and discovery — the contract a plugin must satisfy and the symbols a service looks up are defined by the host application. The manager loads `.so`
files from a directory, resolves their metadata, runs an optional `Init` callback with panic recovery and a timeout, and lets services find providers via
symbol lookup.

> **Security warning.** Loading a `.so` file runs arbitrary native code in the host
> process **before** the manager can inspect the descriptor. A malicious plugin can read
> process memory, open files, make network calls, or call `exit`. Only load plugins from
> directories writable by the deployer alone — never from user uploads, network shares,
> or world-writable locations.

> **Platform note.** Dynamic plugin loading is only supported on `darwin` and `linux`
> (Go's `plugin` package limitation). On other platforms the manager returns
> `ErrUnsupportedPlatform`.

---

## Overview

Main types:

| Type              | Role                                                              |
|-------------------|-------------------------------------------------------------------|
| `Manager`         | Loads, registers, discovers plugins. Owns their lifecycle.       |
| `Plugin`          | A loaded `.so` with state, metadata, and a symbol cache.         |
| `Descriptor`      | Static metadata every plugin exports.                            |
| `DepInfo`         | Optional dependency snapshot for detecting version skew.         |
| `SPIVersion`      | Contract version a plugin provider declares.                     |
| `SPIConstraint`   | Version requirements the host checks providers against.          |
| `SignatureOptions` | Configuration for optional `.so.sig` cryptographic verification. |

What the manager does:

- Scans a directory for `.so` files, filtered by an allowlist (`Load`) or exclusion list (`Disabled`)
- Opens each file, resolves its `Descriptor`, runs the optional `Init` under panic recovery + timeout
- Tracks state atomically: `StateLoaded` → `StateReady` / `StateFailed` → `StateUnloaded`
- Iterates plugins (`Plugins`, `Names`, `Ready`) and discovers providers (`LookupAll`, `NegotiateAll`)
- Warns on Go-version or dependency mismatches between host and plugin (`GoVersion`, `DepInfo`)
- Enforces host major-version compatibility via `Descriptor.HostVersion` (configurable via `HostVersionMode`)
- Verifies detached `.so.sig` signatures before executing any plugin code (optional)
- Quarantines failed plugins by file hash; clears when the file changes
- Optionally watches the directory for new `.so` files at runtime

## When to use

| Scenario                                       | What plugins enable                                                |
|------------------------------------------------|--------------------------------------------------------------------|
| Customer-specific business rules               | Ship the same binary; load tenant-specific `.so` per deployment    |
| Optional commercial features                   | Toggle features without rebuilding the host                        |
| Pluggable provider implementations             | Auth providers, audit sinks, codecs added without core changes     |
| In-house extension marketplace                 | Decouple extension release cadence from host release cadence       |

If you only need *configurable* behavior, use config flags. Plugins make sense when the extension involves Go code that the host should not know about at
compile time.

---

## Plugin contract

A plugin is a Go package compiled with `-buildmode=plugin`. It must export a package-level `Descriptor` variable and may optionally export `Init`.

### Descriptor (required)

Value and pointer forms both work:

```go
package main

import (
    "runtime"

    "github.com/altessa-s/go-atlas/core/plugins"
)

// Value form (recommended).
var Descriptor = plugins.Descriptor{
    Name:        "audit-mongo",
    Version:     "1.2.0",
    Description: "Audit log sink that writes events to MongoDB.",
    GoVersion:   runtime.Version(), // advisory ABI skew detection
}
```

```go
// Pointer form (also supported).
var Descriptor = &plugins.Descriptor{
    Name:      "audit-mongo",
    Version:   "1.2.0",
    GoVersion: runtime.Version(),
}
```

`Name` is the lookup key for `Manager.Get` and must be unique. Empty `Name` → `ErrInvalidDescriptor`.

### Init (optional)

Function and variable forms both work:

```go
// Function declaration (idiomatic Go).
func Init(ctx context.Context) error {
    // one-shot setup work; return error to mark plugin as StateFailed
    return nil
}
```

```go
// Variable form.
var Init = func(ctx context.Context) error {
    return nil
}
```

`Init` runs with a timeout (default 5s) and panic recovery. A panic sets `ErrPluginPanicked`; a returned error sets `ErrPluginFailed`. Either way the plugin
moves to `StateFailed`. Other plugins keep loading — one failure does not stop the rest.

### Service-specific symbols

A plugin can export any additional symbols. The host defines the names it expects and finds them via `Manager.LookupAll`. Each service owns its own SPI
contract.

```go
// Plugin side:
var AuthProvider = &mongoAuthProvider{ /* ... */ }
```

```go
// Host side:
for p, sym := range mgr.LookupAll("AuthProvider") {
    provider, ok := sym.(auth.Provider)
    if !ok {
        log.Warn("plugin exports AuthProvider with unexpected type",
            slog.String("plugin", p.Name()))
        continue
    }
    registry.Register(p.Name(), provider)
}
```

### Building a plugin

```bash
cd ./plugins/audit-mongo
go build -buildmode=plugin -o ../../bin/plugins/audit-mongo.so .
```

Host and plugin must use the same go-atlas version — Go's plugin loader rejects `.so` files compiled against a different version of any shared dependency.

### DepInfo (optional)

Export `DepInfo` so the manager can compare the plugin's module graph against the host's `debug.ReadBuildInfo`. Mismatched shared-dependency versions get a
logged warning.

```go
var DepInfo = plugins.NewDepInfoFromBuild()
```

`NewDepInfoFromBuild` snapshots `debug.ReadBuildInfo` at init time — nothing to maintain. Optional; absent `DepInfo` is silently skipped.

### HostVersion (optional, enforced)

Set `Descriptor.HostVersion` to the host service semver this plugin was built against. When the manager is configured via `WithHostVersion` — the factory
auto-wires it from `core/runtime/appinfo.Version` when the application has been stamped with a non-default version — the loader compares the **major** components
and rejects mismatches.

Unlike `GoVersion` and `DepInfo` (advisory), this check is enforced by default. SemVer guarantees ABI compatibility within a major version, so a plugin built
for v2.x stays compatible with hosts on v2.5.3 but must be rejected on v3.0.0.

```go
var Descriptor = plugins.Descriptor{
    Name:        "audit-mongo",
    Version:     "1.2.0",
    HostVersion: "2.5.0",       // built against the v2.x host contract
    GoVersion:   runtime.Version(),
}
```

Strictness is controlled by `HostVersionMode`:

| Mode                  | On major mismatch                                          |
|-----------------------|------------------------------------------------------------|
| `HostVersionEnforce`  | Return `ErrHostVersionMismatch`, quarantine the plugin     |
| `HostVersionWarn`     | Log a warning, load anyway (canary phase)                  |
| `HostVersionDisabled` | Skip the check entirely (operator kill-switch)             |

The default is `HostVersionEnforce`. The check is skipped silently when either side leaves its version empty, preserving backwards compatibility with plugins
built before this field was added. The host's `appinfo.Version` default `"0.0.0"` also counts as "not set" — the factory does not auto-wire `WithHostVersion`
in dev builds, so `go run ./cmd/service` keeps loading plugins that declare a real `HostVersion`.

### SPI version negotiation

To version a provider contract, export a companion `<Symbol>SPIVersion` variable next to the provider:

```go
var AuthProvider         = &myAuthProvider{}
var AuthProviderSPIVersion = plugins.SPIVersion{
    Contract: "AuthProvider",
    Major:    2,
    Minor:    1,
}
```

On the host side, use `NegotiateAll` instead of `LookupAll` to filter by version:

```go
constraint := plugins.SPIConstraint{Major: 2, MinMinor: 0}
for p, sym := range plugins.NegotiateAll(mgr, "AuthProvider", constraint, logger) {
    provider, ok := sym.(auth.Provider)
    if !ok { continue }
    // version-compatible
}
```

Plugins without a `<Symbol>SPIVersion` symbol pass through unchanged. Incompatible providers are skipped and a warning is logged.

---

## Quick start

```go
import (
    "context"
    "log/slog"
    "os"

    "github.com/altessa-s/go-atlas/core/plugins"
)

func main() {
    ctx := context.Background()

    mgr := plugins.NewManager(
        plugins.WithLogger(slog.New(slog.NewJSONHandler(os.Stdout, nil))),
        plugins.WithDir("./plugins"),
        plugins.WithInitTimeout(10*time.Second),
    )
    defer mgr.Close()

    if err := mgr.Load(ctx); err != nil {
        // Individual failures are joined; successfully loaded plugins stay registered.
        slog.Warn("some plugins failed to load", slog.Any("error", err))
    }

    for name := range mgr.Names() {
        slog.Info("plugin ready", slog.String("plugin", name))
    }
}
```

### Filtering which plugins to load

```go
// Allowlist (takes priority over Disabled): only these files are loaded.
mgr := plugins.NewManager(
    plugins.WithDir("./plugins"),
    plugins.WithLoad("audit-mongo.so", "auth-jwt.so"),
)

// Exclusion list: every .so in the directory is loaded except these.
mgr := plugins.NewManager(
    plugins.WithDir("./plugins"),
    plugins.WithDisabled("experimental-tracer.so"),
)
```

---

## Configuration

YAML config lives under the `plugins` key.

```yaml
plugins:
  enabled: true
  dir: ./plugins
  initTimeout: 5s

  # Allowlist: only these files are loaded (takes priority over disabled).
  load:
    - audit-mongo.so
    - auth-jwt.so

  # Exclusion list (ignored when load is non-empty).
  disabled:
    - experimental-tracer.so

  # Filesystem watcher: pick up new .so files at runtime.
  watch: true
  watchDebounce: 200ms

  # Optional signature verification (see Signature section below).
  signature:
    mode: ""                # "require", "warn", or "" (disabled)
    publicKeyPath: ""       # PEM file with PKIX "PUBLIC KEY" block
```

| Field           | Type            | Default      | Description                                         |
|-----------------|-----------------|--------------|-----------------------------------------------------|
| `enabled`       | `bool`          | `false`      | Master switch; the factory rejects Build when false |
| `dir`           | `string`        | `./plugins`  | Directory scanned for `.so` files                   |
| `load`          | `[]string`      | `[]`         | Allowlist of filenames; takes priority              |
| `disabled`      | `[]string`      | `[]`         | Exclusion list, used only when `load` is empty      |
| `initTimeout`   | `time.Duration` | `5s`         | Per-plugin `Init` timeout                           |
| `watch`         | `bool`          | `false`      | Enable filesystem watcher                           |
| `watchDebounce` | `time.Duration` | `200ms`      | Coalesce window for filesystem events               |
| `signature`     | object          | (disabled)   | Cryptographic `.so.sig` verification; see below     |

A template with comments is at `config/templates/plugins.yaml`.

---

## Factory builder

The `factory` package builds the manager from `config.Plugins`, runs `Load`, and optionally starts the watcher. Nil or disabled configs are rejected at
`Build` time.

```go
import (
    "github.com/altessa-s/go-atlas/core/plugins/factory"
)

mgr, err := factory.NewManager(&cfg.Plugins).
    UseLogger(logger).
    UseHealthCoordinator(healthCoord).
    Build(appCtx)
if err != nil {
    return fmt.Errorf("plugins: %w", err)
}
defer mgr.Close()
```

| Method                                          | Effect                                                            |
|-------------------------------------------------|-------------------------------------------------------------------|
| `NewManager(*config.Plugins)`                   | Constructs the builder; nil config rejected at `Build` time       |
| `UseLogger(*slog.Logger)`                       | Injects the structured logger                                     |
| `UseHealthCoordinator(*health.Coordinator)`     | Registers the manager as a `health.Checker` under the default name `"plugins"` |
| `UseHealthServiceName(string)`                  | Overrides the health service name (no-op for empty input)         |
| `Build(context.Context)`                        | Validates, runs `Load(ctx)`, and optionally `StartWatching(ctx)`. Auto-wires `WithHostVersion(appinfo.Version)` when the binary was stamped with a non-default version. |

> Pass the **application** context to `Build` when `cfg.Watch` is true. The watcher's
> lifetime is bound to that context: when it is canceled, the watch goroutine exits.

---

## Manager API

### Lifecycle

| Method                              | Description                                                              |
|-------------------------------------|--------------------------------------------------------------------------|
| `NewManager(opts ...Option)`        | Constructor; options have functional defaults                            |
| `Load(ctx)`                         | Scans the directory and loads every filtered plugin once                 |
| `Reload(ctx)`                       | Idempotent rescan; loads new files, skips already-loaded ones            |
| `Unload(name)`                      | Removes a plugin from the registry; the underlying `.so` stays in memory |
| `Quarantine(name)`                  | Blacklists a plugin by file hash; removes from registry                  |
| `Close()`                           | Stops the watcher (if running) and clears the registry; idempotent       |
| `StartWatching(ctx)` / `StopWatching()` / `IsWatching()` | See [Filesystem watcher](#filesystem-watcher) below |
| `CheckHealth(ctx)`                  | `health.Checker` implementation: degraded when any plugin is `StateFailed` |
| `LastWatcherReloadErr()`            | Most recent watcher-driven `Reload` error, or nil                        |

### Discovery

| Method                                 | Description                                                          |
|----------------------------------------|----------------------------------------------------------------------|
| `Get(name)`                            | Returns the plugin or `ErrPluginNotFound`                            |
| `MustGet(name)`                        | Panics on missing plugin; for startup or test setup only             |
| `Len()`                                | Number of registered plugins                                         |
| `Plugins()`                            | `iter.Seq2[string, *Plugin]` over all registered plugins             |
| `Names()`                              | `iter.Seq[string]` over all registered plugin names                  |
| `Ready()`                              | `iter.Seq2[string, *Plugin]` filtered to `StateReady` only           |
| `LookupAll(symbol)`                    | `iter.Seq2[*Plugin, any]` over Ready plugins exporting the symbol    |
| `NegotiateAll(mgr, symbol, constraint, logger)` | Like `LookupAll` but filters by `SPIConstraint`; free function |
| `Quarantined()`                        | `iter.Seq2[string, string]` over quarantined files (filename → hash) |

> **Iterator caveat.** All iterator-returning methods yield from a snapshot taken under
> the read lock. The loop body may safely call any Manager method, including write
> methods like `Unload` and `Close`. The snapshot may be slightly stale; re-check
> `Plugin.State()` inside the loop body if strict consistency is needed.

### Plugin accessors

| Method            | Returns                                                              |
|-------------------|----------------------------------------------------------------------|
| `Name()`          | Name from the descriptor                                             |
| `Version()`       | Version from the descriptor                                          |
| `Description()`   | Description from the descriptor                                      |
| `State()`         | Current `State` (atomic)                                             |
| `Path()`          | Filesystem path of the loaded `.so`                                  |
| `LoadedAt()`      | Time the plugin was opened                                           |
| `Err()`           | Last error if `StateFailed`, nil otherwise (atomic)                  |
| `Lookup(symbol)`  | Per-plugin symbol resolution; cached                                 |

### Lifecycle states

```
StateLoaded ──Init success──▶ StateReady ──Unload/Close──▶ StateUnloaded
     │
     └──Init error/panic──▶ StateFailed ──Unload/Close──▶ StateUnloaded
```

`Plugins()` includes every state; `Ready()` and `LookupAll()` skip non-ready plugins.

---

## Quarantine

When a plugin fails to load (broken `.so`, missing descriptor, Init error or panic), the manager records the file's SHA256 hash and skips it on subsequent
`Load`/`Reload` calls. Reloading the same broken file is pointless — nothing changed. The quarantine clears automatically when the file hash changes
(operator deployed a fix).

### Automatic quarantine

Any failure in `loadPlugin` quarantines the file: `openPlugin` error, descriptor resolution, Init error, Init panic. The watcher picks up file changes via
WRITE events and calls `Reload`, which recomputes the hash, clears the quarantine entry, and retries.

### Runtime quarantine

Services that observe a plugin misbehaving at runtime (panics, repeated errors from exported symbols) can quarantine it explicitly:

```go
if err := mgr.Quarantine("broken-plugin"); err != nil {
    log.Error("quarantine failed", slog.Any("error", err))
}
```

The plugin is removed from the registry, marked `StateFailed`, and its file hash is recorded. The next `Reload` skips it unless the file changed.

### Inspecting quarantine

```go
for filename, hash := range mgr.Quarantined() {
    slog.Info("quarantined", slog.String("file", filename), slog.String("hash", hash))
}
```

### Errors

Quarantined plugins produce `ErrPluginQuarantined`, matchable via `errors.Is`.

---

## Signature verification

Verifies `.so` files against a detached `.so.sig` signature before `plugin.Open` runs any code. Disabled by default. When on, each `.so` needs a companion
`.sig` file signed with the configured public key.

> **TOCTOU note.** `plugin.Open` re-reads the file from disk — it doesn't accept
> pre-read bytes. An attacker with write access to the plugin directory could swap the
> `.so` between verification and `Open`. The primary defense is filesystem permissions:
> the plugin directory must be writable only by the deployer. See the Security section.

### Configuration

```yaml
plugins:
  enabled: true
  dir: /opt/myservice/plugins
  signature:
    mode: require          # "require", "warn", or "" (disabled)
    publicKeyPath: /etc/myservice/plugin-signing-key.pub
```

| Field           | Type     | Default | Description                                     |
|-----------------|----------|---------|-------------------------------------------------|
| `mode`          | `string` | `""`    | `"require"` rejects unsigned, `"warn"` allows   |
| `publicKeyPath` | `string` | `""`    | PEM file with PKIX "PUBLIC KEY" block            |

### Algorithms

| Algorithm    | Key type             | `.sig` format       | Size      |
|--------------|----------------------|---------------------|-----------|
| Ed25519      | `ed25519.PublicKey`   | raw bytes           | 64 bytes  |
| ECDSA P-256  | `*ecdsa.PublicKey`    | ASN.1 DER           | ~70 bytes |
| RSA-PSS      | `*rsa.PublicKey`      | raw PSS signature   | key/8     |

Ed25519 signs the raw `.so` bytes. ECDSA and RSA sign the SHA-256 digest.

### Modes

- **`require`** / **`enforce`**: plugin without valid `.sig` → `ErrSignatureMissing`,
  quarantined.
- **`warn`**: missing `.sig` → warning log, plugin loads anyway. Invalid signature →
  still rejected and quarantined.
- **`""` (default)**: no verification.

### Signing a plugin (offline)

```bash
# Ed25519:
openssl genpkey -algorithm Ed25519 -out plugin-key.pem
openssl pkey -in plugin-key.pem -pubout -out plugin-key.pub

# Sign:
openssl pkeyutl -sign -inkey plugin-key.pem -rawin \
    -in myplugin.so -out myplugin.so.sig
```

### Programmatic use

```go
mgr := plugins.NewManager(
    plugins.WithDir("./plugins"),
    plugins.WithSignature(plugins.SignatureOptions{
        Mode:      plugins.SignatureRequire,
        PublicKey:  myEd25519PublicKey, // or use PublicKeyPath for PEM
    }),
)
```

### Errors

| Error                 | When                                            |
|-----------------------|-------------------------------------------------|
| `ErrSignatureInvalid` | `.sig` exists but does not verify                |
| `ErrSignatureMissing` | `.sig` not found and mode is `require`           |
| `ErrSignatureConfig`  | Bad PEM, unsupported key type, missing key path  |

---

## Filesystem watcher

With `config.Plugins.Watch = true` (or `Manager.StartWatching` directly), an [`fsnotify`](https://github.com/fsnotify/fsnotify) watcher monitors the plugin
directory and loads new `.so` files as they appear.

### Semantics

- **Additions only.** Modifications and removals are ignored — Go's `plugin` package
  cannot unload or replace mapped code. Upgrading a plugin requires restarting the host.
- **Debounced.** Rapid filesystem events (e.g. `rsync` of several files) are coalesced
  into one `Reload` after `watchDebounce` (default 200ms).
- **Best-effort.** A failed `Reload` is logged; the watcher keeps running.
- **Context-bound.** Canceling the context passed to `StartWatching` stops the watcher.

### Programmatic control

```go
// Start manually with the application context.
if err := mgr.StartWatching(appCtx); err != nil {
    return fmt.Errorf("start watcher: %w", err)
}

// Inspect.
if mgr.IsWatching() {
    log.Info("plugin watcher active")
}

// Stop manually (Close() also stops the watcher).
mgr.StopWatching()
```

When `cfg.Watch = true` the factory calls `StartWatching(ctx)` after the initial `Load`.

---

## Sandbox mode

The manager can apply Linux process-hardening primitives before opening any `.so` file. This reduces blast radius for buggy plugins — it is not isolation.
Go's `plugin` package loads native code into the host address space, so in-process isolation is impossible.

> **Read this before enabling.**
>
> - **Linux only.** On other platforms an enabled sandbox makes `Manager.Load` fail
>   with `ErrSandboxUnsupported`.
> - **Lazy and irreversible.** Applied on the first `Load`. Host code that runs before
>   the first `Load` is unrestricted; from that point onward the limits apply for the
>   rest of the host's lifetime.
> - **rlimits and seccomp are process-wide.** `setrlimit(2)` affects the whole process.
>   Setting `maxOpenFiles: 256` will starve your database pool. These are **not**
>   plugin-scoped.
> - **NoNewPrivs, capabilities, and Landlock are per-thread in Go.**
>   These syscalls only restrict the calling thread. The Go runtime already has
>   several OS threads (sysmon, GC, netpoll, workers) when the sandbox runs, and
>   peer threads keep their original state. The manager logs a `WARN` about this.
>   For process-wide isolation, drop privileges externally; see
>   [Recommended deployment](#recommended-deployment).
> - **Not isolation.** A malicious plugin still has full access to host memory.
>   The sandbox catches accidental misbehavior, not a determined attacker.

### Primitives

| Primitive                  | Mechanism                                  | Effect when set                                          |
|----------------------------|--------------------------------------------|----------------------------------------------------------|
| `noNewPrivs`               | `prctl(PR_SET_NO_NEW_PRIVS, 1)`            | The process and any binary it execs cannot gain privileges via SUID/SGID |
| `memoryLimitBytes`         | `setrlimit(RLIMIT_AS, n)`                  | Caps total virtual address space; `mmap`/allocator failures past `n`    |
| `maxOpenFiles`             | `setrlimit(RLIMIT_NOFILE, n)`              | Caps total open file descriptors                                         |
| `maxProcesses`             | `setrlimit(RLIMIT_NPROC, n)`               | Caps subprocess creation                                                 |
| `maxFileSizeBytes`         | `setrlimit(RLIMIT_FSIZE, n)`               | Caps the maximum size of any file the process writes                     |
| `disableCoreDumps`         | `setrlimit(RLIMIT_CORE, 0)`                | Suppresses core dumps so memory contents (including secrets) cannot leak |
| `landlock`                 | `landlock_create_ruleset` + `add_rule` + `restrict_self` | Strict filesystem allowlist enforced by the kernel       |

`seccomp-BPF` is in `core/runtime/seccomp` but is **not** wired into the plugin sandbox. Call
`seccomp.BlockDangerousSyscalls()` during host startup before constructing the manager. The denylist is fixed, not operator-configurable, to keep the
audited set stable.

### Configuration

```yaml
plugins:
  enabled: true
  dir: /opt/myservice/plugins
  sandbox:
    enabled: true
    noNewPrivs: true
    memoryLimitBytes: 268435456  # 256 MiB total host RAM cap
    maxOpenFiles: 4096           # must be larger than the host pool
    maxProcesses: 64
    maxFileSizeBytes: 1073741824 # 1 GiB
    disableCoreDumps: true
```

### Field reference

| Field              | Type     | Default | Host-side cost                                                          |
|--------------------|----------|---------|-------------------------------------------------------------------------|
| `enabled`          | `bool`   | `false` | Master switch                                                           |
| `noNewPrivs`       | `bool`   | `true`  | None; has no effect on a non-setuid host                                |
| `memoryLimitBytes` | `int64`  | `0`     | **High.** Counts every allocation, including the Go runtime and pools   |
| `maxOpenFiles`     | `int64`  | `0`     | **High.** Counts every fd: DB pool, HTTP server, Redis, epoll, sockets  |
| `maxProcesses`     | `int64`  | `0`     | **High.** `RLIMIT_NPROC` is per real UID, not per process (see note below) |
| `maxFileSizeBytes` | `int64`  | `0`     | Medium. Affects log files, snapshots, exported reports                  |
| `disableCoreDumps` | `bool`   | `false` | None; operationally beneficial; loses post-crash forensic data          |
| `capabilities`     | object   | (off)   | Medium. Dropping caps the host legitimately needs breaks startup        |
| `landlock`         | object   | (off)   | **Critical.** Misconfigured allowlist breaks dlopen and the host        |

`0` always means "unset" (kernel default). Negative values are rejected by `Validate`.

> **`maxProcesses` is per real UID, not per process.** The Linux
> `RLIMIT_NPROC` cap counts every process owned by the same real UID,
> including unrelated processes started by other binaries running under
> the same user account. On a host running multiple service instances
> under one UID, or running as a shared system user (`nobody`,
> `daemon`), a low `maxProcesses` value can starve unrelated processes,
> and the symptom (`fork: Resource temporarily unavailable` from a
> sibling process) does not point back to this rlimit. Run the service
> under a dedicated UID, or leave `maxProcesses: 0` and bound process
> count via systemd `TasksMax=` / cgroup `pids.max` instead. Both are
> per-cgroup rather than per-UID.

### Capability dropping

Plugins loaded via `dlopen` inherit the host's Linux capabilities. Without dropping, a plugin could craft raw packets, bind privileged ports, bypass DAC,
trace processes, or mount filesystems.

The capability-dropping pass runs between `rlimits` and Landlock, using `core/runtime/capabilities` (raw
`capset(2)` + `prctl(2)`, no cgo).

**Configuration:**

```yaml
plugins:
  enabled: true
  sandbox:
    enabled: true
    capabilities:
      enabled: true
      # Leave keep empty to drop every capability.
      keep: []
```

To preserve specific capabilities (typical "bind :443 then drop the rest" pattern), list them in `keep`:

```yaml
    capabilities:
      enabled: true
      keep:
        - CAP_NET_BIND_SERVICE
```

Names match kernel `CAP_*` constants (case-insensitive). Unknown names fail `config.Validate` at load time, not at `Manager.Load`.

> **Read this before enabling.**
>
> - **Linux only.** On non-Linux platforms an enabled
>   `capabilities` block makes the manager fail `Manager.Load` with
>   `ErrSandboxFailed` wrapping `ErrSandboxUnsupported`.
> - **Per-thread, not process-wide.** `capset(2)` only restricts the calling thread.
>   For process-wide dropping, use systemd or a container runtime; see
>   [Recommended deployment](#recommended-deployment).
> - **Irreversible.** Once dropped on a thread, capabilities cannot be raised back.
> - **Empty `keep` = drop everything.** If the host needs a capability after startup,
>   list it in `keep` explicitly.
> - **Inheritable and ambient sets are always cleared.** Plugin hosts don't need to
>   propagate capabilities across `execve`.
> - **Requires `sandbox.enabled = true`.** Sub-feature of the umbrella sandbox.

### Landlock filesystem allowlist

Landlock (Linux LSM, kernel 5.13+) lets a process restrict its own filesystem access without root. The sandbox uses it as a strict allowlist: anything not
listed returns `EACCES`.

> **Read this before enabling.**
>
> - **Linux 5.13+ only.** On older kernels `landlock_create_ruleset` returns `ENOSYS`
>   and the manager fails Load with `ErrSandboxFailed`.
> - **Strict allowlist.** Nothing is auto-added by default. The operator must list
>   every path the host needs.
> - **Misconfiguration is silent.** A missing entry shows up as "permission denied"
>   from `openat`/`execve`, usually inside `dlopen` trying to load `ld-linux` or
>   `libc.so.6`. There is no better error message.
> - **NNP is set automatically.** `landlock_restrict_self` requires NNP or
>   `CAP_SYS_ADMIN`. `landlock.Apply` probes ABI support first; on supported
>   kernels it sets NNP before constructing the ruleset. On unsupported kernels
>   NNP is not set. The `noNewPrivs` field above only matters when Landlock is off.
> - **Per-thread in Go.** Same limitation as other per-thread primitives.
>   See [Recommended deployment](#recommended-deployment) for process-wide options.
> - **Irreversible per thread.**

#### ABI versions

| ABI | Kernel | Adds                                |
|-----|--------|-------------------------------------|
| 1   | 5.13   | `read_file`, `read_dir`, `write_file`, `execute`, `remove_*`, `make_*` |
| 2   | 5.19   | `refer` (cross-path rename/link)    |
| 3   | 6.2    | `truncate`                          |
| 4   | 6.7    | `ioctl_dev`                         |
| 5   | 6.10   | scoped signals                      |
| 6   | 6.12   | additional restrictions             |

The implementation negotiates the highest ABI the kernel supports and adds `truncate` to the write mask on v3+. Older kernels degrade to v1. Log rotation
using truncate may break — verify on a canary.

#### Configuration

```yaml
plugins:
  enabled: true
  dir: /opt/myservice/plugins
  sandbox:
    enabled: true
    noNewPrivs: true              # implicitly forced by landlock anyway
    landlock:
      enabled: true
      allowPluginDir: true        # auto-include plugins.dir in readPaths
      allowSystemLibs: true       # auto-include /lib, /lib64, /usr/lib, /usr/lib64
      readPaths:
        - /etc/myservice          # read-only config
      readWritePaths:
        - /var/lib/myservice      # data directory
        - /var/log/myservice      # log directory
```

| Field             | Type       | Default | Description                                       |
|-------------------|------------|---------|---------------------------------------------------|
| `enabled`         | `bool`     | `false` | Turn Landlock on                                  |
| `allowPluginDir`  | `bool`     | `false` | Auto-add `plugins.dir` to `readPaths`             |
| `allowSystemLibs` | `bool`     | `false` | Auto-add `/lib`, `/lib64`, `/usr/lib`, `/usr/lib64` to `readPaths` |
| `readPaths`       | `[]string` | `[]`    | Additional paths granted read+execute. Absolute paths only        |
| `readWritePaths`  | `[]string` | `[]`    | Paths granted read+execute+write+truncate. Absolute paths only    |

#### Auto-added paths

Two convenience flags merge common paths into `readPaths` at apply time. The original slice is cloned before merging.

##### `allowPluginDir`

Appends `plugins.dir` to `readPaths`. Almost every deployment needs this — without it `dlopen` can't read the `.so`. Off by default only to preserve the
strict-allowlist contract for audits that require every path to be explicit.

##### `allowSystemLibs`

Appends these paths to `readPaths`:

- `/lib`
- `/lib64`
- `/usr/lib`
- `/usr/lib64`

Covers glibc and musl on RHEL, Debian, Ubuntu, Fedora, Alpine, and most mainstream distros. It's a blunt instrument:

- Grants `read+execute` on everything under those directories, not just what the
  plugin needs.
- Does **not** include multiarch subdirs (`/lib/x86_64-linux-gnu`). On Debian/Ubuntu
  these are usually reachable via traversal from `/lib`, but if your loader lives elsewhere, add the path to `readPaths` explicitly.
- **Not suitable for** NixOS (libs under `/nix/store`), chroot jails, or custom
  library prefixes. Leave `allowSystemLibs` off and list `ldd` output paths manually.

When in doubt: `allowSystemLibs` off, `ldd your-plugin.so`, copy the paths.

##### Strict mode

Every path explicit (for audits):

```yaml
landlock:
  enabled: true
  allowPluginDir: false     # audit requires explicit listing
  allowSystemLibs: false
  readPaths:
    - /opt/myservice/plugins
    - /lib64
    - /usr/lib64
    - /etc/myservice
  readWritePaths:
    - /var/lib/myservice
    - /var/log/myservice
```

##### Auto-add

Minimum config for a glibc host:

```yaml
landlock:
  enabled: true
  allowPluginDir: true
  allowSystemLibs: true
  readPaths:
    - /etc/myservice
  readWritePaths:
    - /var/lib/myservice
    - /var/log/myservice
```

Both forms produce identical rulesets on a standard glibc system.

#### Minimum-viable allowlist by distribution

Exact paths depend on libc and dynamic loader. Starting points below — verify on the actual deployment image.

**glibc x86_64 (RHEL, Debian, Ubuntu, Fedora):**

```yaml
readPaths:
  - /opt/myservice/plugins   # plugin directory
  - /lib64                   # ld-linux-x86-64.so.2 + libc.so.6
  - /usr/lib64               # additional system libs (libdl, libpthread, libm, ...)
  - /etc/ld.so.cache         # dynamic loader cache (some distros require it)
```

**glibc arm64 (RHEL, Debian, Ubuntu):**

```yaml
readPaths:
  - /opt/myservice/plugins
  - /lib                     # ld-linux-aarch64.so.1 + libc.so.6 on arm64
  - /usr/lib
  - /etc/ld.so.cache
```

**musl Alpine:**

```yaml
readPaths:
  - /opt/myservice/plugins
  - /lib                     # ld-musl-x86_64.so.1
  - /usr/lib                 # additional libs
```

**Debian/Ubuntu multiarch (architecture-specific subdirectories):**

```yaml
readPaths:
  - /opt/myservice/plugins
  - /lib                     # /lib/ld-linux-x86-64.so.2 symlink
  - /lib/x86_64-linux-gnu    # actual loader + libc
  - /usr/lib/x86_64-linux-gnu
```

#### Debugging permission denied errors

Plugin fails to load with `permission denied`? Usually the dynamic loader or a shared library. Trace file accesses on a host without Landlock:

```bash
strace -f -e trace=openat,execve ./myservice 2>&1 \
  | grep -E '(openat|execve).*EACCES|/lib|/usr/lib|/etc/ld'
```

Add missing paths to `readPaths`.

For stricter analysis:

```bash
ldd /opt/myservice/plugins/audit-mongo.so
```

Any `ldd` output pointing outside `/lib`/`/usr/lib` needs an explicit allowlist entry.

#### Failure semantics

Any failure during ABI detection, ruleset creation, or `restrict_self` wraps in `ErrSandboxFailed` and stops `Manager.Load`. No fallback — a partial
Landlock setup would be worse than none. The error is cached and returned by every subsequent `Load` and `Reload`. `CheckHealth` reports `StatusNotServing`.

#### Limitations

- **Per-thread, not process-wide.** Same as other per-thread primitives. See
  [Recommended deployment](#recommended-deployment) for process-wide options.
- **Per-process, not per-plugin.** Per-plugin isolation would need an out-of-process
  worker model.
- **Symlinks:** the kernel checks the resolved target, not the link. Think in terms
  of resolved inodes, not link locations.
- Uses access flags up to ABI v3 (`truncate`). v4+ flags are not used yet.

#### Using the hardening primitives outside the plugin manager

The plugin sandbox wraps four standalone packages. Any service can use them directly:

| Package | What it does |
|---|---|
| `github.com/altessa-s/go-atlas/core/runtime/nonewprivs` | `PR_SET_NO_NEW_PRIVS`: defeats SUID/SGID escalation on exec |
| `github.com/altessa-s/go-atlas/core/runtime/rlimits` | `setrlimit(2)`: caps memory, FDs, processes, file size, core dumps |
| `github.com/altessa-s/go-atlas/core/runtime/capabilities` | `capset(2)` / `prctl(2)`: drops Linux capabilities so plugins can't inherit privileges |
| `github.com/altessa-s/go-atlas/core/runtime/landlock` | Linux Landlock LSM filesystem allowlist (kernel 5.13+) |

Each has a single entry point, `ErrUnsupported`/`ErrFailed` sentinels, and a non-Linux stub for cross-platform builds.

```go
import (
    "github.com/altessa-s/go-atlas/core/runtime/capabilities"
    "github.com/altessa-s/go-atlas/core/runtime/landlock"
    "github.com/altessa-s/go-atlas/core/runtime/nonewprivs"
    "github.com/altessa-s/go-atlas/core/runtime/rlimits"
)

// At service startup, before any long-running work. Apply the primitives
// in this order: nonewprivs → rlimits → capabilities → landlock.
//
// Note: landlock.Apply probes ABI support first (a side-effect-free
// read), and only calls nonewprivs.Set() internally if the kernel
// supports Landlock. Calling nonewprivs.Set() here is therefore
// optional on Landlock-capable kernels but is the only way to commit
// to NNP on kernels without Landlock — keep it explicit.
if err := nonewprivs.Set(); err != nil {
    log.Fatalf("nonewprivs: %v", err)
}
if err := rlimits.Apply(
    rlimits.WithMemoryBytes(512 << 20),
    rlimits.WithMaxOpenFiles(4096),
    rlimits.WithDisableCoreDumps(),
); err != nil {
    log.Fatalf("rlimits: %v", err)
}
// Drop every capability except CAP_NET_BIND_SERVICE — typical pattern
// for a service that bound :443 before this point.
if err := capabilities.DropAllExcept(capabilities.CAP_NET_BIND_SERVICE); err != nil {
    log.Fatalf("capabilities: %v", err)
}
if err := landlock.Apply(
    landlock.WithReadPaths("/etc/myservice", "/lib64", "/usr/lib64"),
    landlock.WithReadWritePaths("/var/lib/myservice", "/var/log/myservice"),
); err != nil {
    log.Fatalf("landlock: %v", err)
}
```

The plugin sandbox composes all four and adds `allowPluginDir`/`allowSystemLibs` convenience flags plus the once-cached state machine. See each package's
README for the standalone API.

### Programmatic configuration

When using the manager directly (without the factory), populate `SandboxOptions`:

```go
mgr := plugins.NewManager(
    plugins.WithDir("./plugins"),
    plugins.WithSandbox(plugins.SandboxOptions{
        Enabled:          true,
        NoNewPrivs:       true,
        MemoryLimitBytes: 256 << 20,
        MaxOpenFiles:     4096,
        DisableCoreDumps: true,
    }),
)

if err := mgr.Load(ctx); err != nil {
    return err // wraps ErrSandboxFailed if any primitive failed
}
```

`plugins.SandboxOptionsFromConfig(cfg.Plugins.Sandbox)` converts the YAML config into the runtime struct.

### Operational guidance

**Before enabling in production:**

1. Profile steady-state resource usage. Set `maxOpenFiles` and `memoryLimitBytes`
   2-3x above observed peak.
2. Test failure modes. Too few fds = `EMFILE` from `accept()`/`open()`.
3. Roll out on a canary first.

**Verifying the sandbox:**

```bash
# Find the host PID and inspect its rlimits.
cat /proc/$(pidof myservice)/limits
```

Both soft and hard limits will show the configured value (the manager sets `Cur == Max`).

**When the sandbox blocks something legitimate:**

There's no "unsetrlimit". Restart with corrected config. Treat sandbox values like resource requests — same change-control track.

**Failure semantics:**

Once any primitive fails, `Load` and `Reload` return the cached error on every call. A half-applied sandbox is worse than none at all, so the manager
refuses to load plugins until the config is fixed or the host is restarted.

### Recommended deployment

This is the runbook the per-thread `WARN` log points to.

**TL;DR:** don't rely on the in-process sandbox alone for `noNewPrivs`, `capabilities`, or `landlock`. Drop those privileges externally before the Go binary
starts. The in-process call is defense-in-depth, not the primary boundary.

#### Why per-thread is a problem in Go

`PR_SET_NO_NEW_PRIVS`, `capset(2)`, and `landlock_restrict_self(2)` are per-thread on Linux. By the time `main()` runs, the Go runtime already has several
OS threads (sysmon, GC, netpoll, workers) that aren't addressable from Go. The sandbox syscalls land on the current thread; peer threads keep their original
state.

`rlimits` and `seccomp` (with `TSYNC`) are real process-wide — they're not affected.

#### Option A: systemd unit (recommended for bare-metal hosts)

```ini
[Unit]
Description=My Service
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/myservice
User=myservice
Group=myservice

# --- Process-wide privilege drop applied by systemd before exec(2) ---
NoNewPrivileges=yes

# Drop all Linux capabilities. List exceptions explicitly:
CapabilityBoundingSet=
AmbientCapabilities=
# Example: keep only CAP_NET_BIND_SERVICE for binding :443
# CapabilityBoundingSet=CAP_NET_BIND_SERVICE
# AmbientCapabilities=CAP_NET_BIND_SERVICE

# Filesystem allowlist via systemd's Landlock-style directives.
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/lib/myservice /var/log/myservice
ReadOnlyPaths=/etc/myservice /opt/myservice/plugins

# Resource limits (these are also process-wide via setrlimit, so the
# plugin sandbox's rlimits block can stay enabled as a redundant layer).
LimitNOFILE=4096
LimitNPROC=64
LimitCORE=0

# Optional: seccomp denylist via systemd. Complements
# seccomp.BlockDangerousSyscalls() called from the host's main().
SystemCallFilter=@system-service
SystemCallFilter=~@mount @reboot @swap @raw-io @debug
SystemCallErrorNumber=EPERM

[Install]
WantedBy=multi-user.target
```

The kernel applies all restrictions before `execve(2)`. Every thread the Go runtime creates inherits them. The in-process sandbox becomes optional
defense-in-depth.

#### Option B: container runtime (recommended for containerized deployments)

Docker / Podman / containerd / CRI-O all expose these primitives:

```bash
docker run \
    --user myservice:myservice \
    --read-only \
    --tmpfs /tmp:size=64m \
    --volume /var/lib/myservice:/var/lib/myservice:rw \
    --volume /var/log/myservice:/var/log/myservice:rw \
    --volume /etc/myservice:/etc/myservice:ro \
    --cap-drop=ALL \
    --cap-add=NET_BIND_SERVICE \
    --security-opt=no-new-privileges \
    --security-opt=seccomp=/etc/myservice/seccomp-profile.json \
    --ulimit nofile=4096:4096 \
    --ulimit nproc=64:64 \
    --ulimit core=0:0 \
    myservice:latest
```

Kubernetes equivalent (`securityContext`):

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: myservice
          image: myservice:latest
          securityContext:
            allowPrivilegeEscalation: false  # PR_SET_NO_NEW_PRIVS
            readOnlyRootFilesystem: true
            runAsNonRoot: true
            capabilities:
              drop: ["ALL"]
              add: ["NET_BIND_SERVICE"]
            seccompProfile:
              type: RuntimeDefault
          resources:
            limits:
              memory: 512Mi
              cpu: "2"
```

#### Option C: tiny C launcher (bare-metal without systemd)

Without systemd or a container, a small C launcher drops privileges and `execve`s the Go binary. Since `execve` happens before any Go thread exists, all
threads inherit the restrictions.

```c
// launcher.c — drop_and_exec.c
// Build: gcc -O2 -Wall -o myservice-launcher launcher.c -lcap
//        sudo setcap cap_setpcap+ep ./myservice-launcher  # if needed
#define _GNU_SOURCE
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/prctl.h>
#include <sys/resource.h>
#include <unistd.h>
#include <linux/landlock.h>
#include <sys/syscall.h>
#include <fcntl.h>

static void die(const char *what) {
    fprintf(stderr, "launcher: %s: %s\n", what, strerror(errno));
    exit(1);
}

int main(int argc, char **argv, char **envp) {
    // 1. NO_NEW_PRIVS — required for unprivileged seccomp / Landlock.
    if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) != 0) die("prctl NNP");

    // 2. Drop the capability bounding set.
    for (int cap = 0; cap < 64; cap++) {
        if (prctl(PR_CAPBSET_DROP, cap, 0, 0, 0) != 0 && errno != EINVAL)
            die("PR_CAPBSET_DROP");
    }

    // 3. rlimits.
    struct rlimit nofile = { 4096, 4096 };
    if (setrlimit(RLIMIT_NOFILE, &nofile) != 0) die("RLIMIT_NOFILE");
    struct rlimit core = { 0, 0 };
    if (setrlimit(RLIMIT_CORE, &core) != 0) die("RLIMIT_CORE");

    // 4. Landlock — example: read-only /usr, RW /var/lib/myservice.
    //    Omitted for brevity; see kernel docs and the full launcher
    //    template in /usr/share/myservice/launcher.c.

    // 5. Hand off to the Go binary. Every thread it creates inherits
    //    the restrictions above because they were applied before exec.
    execve("/usr/local/bin/myservice", argv, envp);
    die("execve");
    return 1;  // unreachable
}
```

This is the only way to get truly process-wide Landlock with Go: the launcher has one thread when it calls `landlock_restrict_self`.

#### When is the in-process sandbox useful?

Even with external deployment, the in-process sandbox adds:

- `rlimits` as a redundant layer (cheap, idempotent).
- `seccomp.BlockDangerousSyscalls()` as a complement to the systemd/container profile.
- `nonewprivs`/`capabilities`/`landlock` as best-effort defense-in-depth on the
  threads they reach. They catch bugs, not deliberate exploitation.
- Fail-closed: misconfigured sandbox (bad rlimit, unknown capability) fails
  `Manager.Load` instead of running with broken hardening.

If you can't deploy A/B/C, leave the per-thread primitives off and use only `rlimits` + `seccomp`. Better to be honest about what's actually enforced.

---

## Concurrency

| Property                              | Guarantee                                                       |
|---------------------------------------|-----------------------------------------------------------------|
| `Manager` public methods              | Concurrent-safe                                                 |
| `Plugin.State()` / `Err()`            | Atomic reads from any goroutine                                 |
| `Plugin.Lookup`                       | Concurrent-safe; cached in `sync.Map`                           |
| Iterators (`Plugins`, `Ready`, etc.)  | Yield from a snapshot; loop body may call any Manager method    |
| `Init` callback                       | Panic recovery + per-plugin timeout                             |
| Concurrent `Load` calls               | Safe but redundant; loser gets `ErrPluginAlreadyLoaded`         |
| Watcher start/stop                    | Idempotent                                                      |

---

## Errors

Sentinel errors in `core/plugins/errors.go`, all matchable via `errors.Is`.

| Error                       | Meaning                                                            |
|-----------------------------|--------------------------------------------------------------------|
| `ErrPluginNotFound`         | `Get`/`Unload` called with an unknown name                         |
| `ErrPluginAlreadyLoaded`    | A plugin with the same `Descriptor.Name` is already registered     |
| `ErrPluginFailed`           | `Init` returned a non-nil error                                    |
| `ErrPluginPanicked`         | `Init` panicked; stack trace logged via the panic recovery handler |
| `ErrUnsupportedPlatform`    | Running on a platform without Go plugin support (non darwin/linux) |
| `ErrNoDescriptor`           | The `.so` does not export a `Descriptor` symbol                    |
| `ErrInvalidDescriptor`      | Descriptor has an unsupported type or empty `Name`                 |
| `ErrInvalidInit`            | `Init` symbol exists but has an unsupported signature              |
| `ErrInvalidDepInfo`         | `DepInfo` symbol exists but has an unsupported type                |
| `ErrSignatureInvalid`       | `.sig` exists but does not verify against the public key           |
| `ErrSignatureMissing`       | `.sig` not found and mode is `require`/`enforce`                   |
| `ErrSignatureConfig`        | Bad PEM, unsupported key type, or missing public key path          |
| `ErrPluginQuarantined`      | Plugin file is blacklisted; same hash as a previous failed load    |
| `ErrSPIVersionMismatch`     | Plugin's SPI version does not satisfy the host's `SPIConstraint`   |
| `ErrHostVersionMismatch`    | Plugin's `Descriptor.HostVersion` major differs from the host major (Enforce mode) |
| `ErrManagerClosed`          | Method called on a closed manager                                  |
| `ErrDirNotFound`            | Configured plugin directory does not exist                         |
| `ErrSandboxUnsupported`     | Sandbox enabled on a non-Linux platform                            |
| `ErrSandboxFailed`          | Applying sandbox primitives failed; wraps the underlying syscall error |

`Load` and `Reload` join individual failures via `errors.Join`. Successful plugins stay registered even when others fail.

---

## Health check

`Manager` implements `observability/health.Checker` with three states:

| Status | When |
|---|---|
| `StatusServing` | No plugin is `StateFailed` (empty registry counts as serving). |
| `StatusDegraded` | Some plugins failed, but at least one is still working. |
| `StatusNotServing` | Manager closed, sandbox failed, or **every** plugin is `StateFailed`. |

`StatusDegraded` matters for readiness probes — you don't want to kill a replica because one plugin broke. Most operators wire it to "still ready, but page
on-call".

Register via `factory.UseHealthCoordinator`.

---

## Operational guidance

### Directory layout

Typical layout:

```
/opt/myservice/
├── bin/
│   └── myservice              # the host binary
└── plugins/
    ├── audit-mongo.so
    ├── audit-mongo.so.sig     # detached signature (when signature.mode is set)
    ├── auth-jwt.so
    ├── auth-jwt.so.sig
    ├── tracing-otlp.so
    └── tracing-otlp.so.sig
```

Lock down ownership:

```bash
chown -R root:root /opt/myservice/plugins
chmod 755 /opt/myservice/plugins
chmod 644 /opt/myservice/plugins/*.so
```

### Graceful shutdown

`Manager.Close` is idempotent and stops the watcher:

```go
defer func() {
    if err := mgr.Close(); err != nil {
        slog.Error("close plugin manager", slog.Any("error", err))
    }
}()
```

### Upgrading a plugin

Go's `plugin` package cannot unload a `.so` once opened. To upgrade:

1. Stop the host.
2. Replace the `.so`.
3. Start the host.

Rolling restarts behind a load balancer keep the service up. The watcher does not help here — modifying an already-loaded `.so` has no effect on the running
process.

### Failure isolation

Init panics are recovered. The plugin moves to `StateFailed`, other plugins keep loading. Health degrades automatically, and the joined error from `Load`
carries the panic message.

### Logging

Log levels:

- `Info`: lifecycle transitions (load, unload, signature verified, quarantine cleared, watcher start/stop). Quiet at steady state.
- `Debug`: per-event filesystem activity, quarantined plugin skips. Chatty — leave off in production.
- `Warn`: Go-version mismatch, dependency skew, SPI version incompatibility, bad `Init` type, missing `.sig` in warn mode, plugin quarantined.
- `Error`: per-plugin load failures from the watcher's `Reload`.

A failure surfaces either in the `Load` return value or in a watcher `Error` log, not both.

---

## API reference

### Types

```go
type Manager struct{ /* opaque */ }
type Plugin struct{ /* opaque */ }

type Descriptor struct {
    Name        string
    Version     string
    Description string
    GoVersion   string // advisory; compared against runtime.Version() at load time
    HostVersion string // enforced; major compared against the host service semver when both sides declare
}

type HostVersionMode int
const (
    HostVersionEnforce HostVersionMode = iota // default; mismatch returns ErrHostVersionMismatch
    HostVersionWarn                           // log and load anyway
    HostVersionDisabled                       // skip the check entirely
)

type DepInfo struct {
    GoVersion string
    Deps      []ModDep
}

type ModDep struct {
    Path    string
    Version string
}

type SPIVersion struct {
    Contract string
    Major    int
    Minor    int
}

type SPIConstraint struct {
    Major    int
    MinMinor int
}

type SignatureOptions struct {
    Mode          SignatureMode
    PublicKey     crypto.PublicKey
    PublicKeyPath string
}

type SignatureMode string
const (
    SignatureDisabled SignatureMode = ""
    SignatureRequire  SignatureMode = "require"
    SignatureEnforce  SignatureMode = "enforce"
    SignatureWarn     SignatureMode = "warn"
)

type State int
const (
    StateLoaded State = iota
    StateReady
    StateFailed
    StateUnloaded
)
```

### Constructor and options

```go
func NewManager(opts ...Option) *Manager

func WithLogger(*slog.Logger) Option
func WithDir(string) Option
func WithLoad(...string) Option
func WithDisabled(...string) Option
func WithInitTimeout(time.Duration) Option
func WithWatchDebounce(time.Duration) Option
func WithHostVersion(string) Option
func WithHostVersionMode(HostVersionMode) Option
func WithSandbox(SandboxOptions) Option
func WithSignature(SignatureOptions) Option
```

### Free functions

```go
func NewDepInfoFromBuild() *DepInfo
func NegotiateAll(mgr *Manager, symbol string, constraint SPIConstraint, logger *slog.Logger) iter.Seq2[*Plugin, any]
func SandboxOptionsFromConfig(config.PluginsSandbox) SandboxOptions
func SignatureOptionsFromConfig(mode, publicKeyPath string) SignatureOptions
```

### Manager methods

```go
func (m *Manager) Load(ctx context.Context) error
func (m *Manager) Reload(ctx context.Context) error
func (m *Manager) Get(name string) (*Plugin, error)
func (m *Manager) MustGet(name string) *Plugin
func (m *Manager) Unload(name string) error
func (m *Manager) Close() error
func (m *Manager) Len() int
func (m *Manager) Plugins() iter.Seq2[string, *Plugin]
func (m *Manager) Names() iter.Seq[string]
func (m *Manager) Ready() iter.Seq2[string, *Plugin]
func (m *Manager) LookupAll(symbol string) iter.Seq2[*Plugin, any]
func (m *Manager) StartWatching(ctx context.Context) error
func (m *Manager) StopWatching()
func (m *Manager) IsWatching() bool
func (m *Manager) CheckHealth(ctx context.Context) health.ServingStatus
func (m *Manager) LastWatcherReloadErr() error
func (m *Manager) Quarantine(name string) error
func (m *Manager) Quarantined() iter.Seq2[string, string]
```

### Plugin methods

```go
func (p *Plugin) Name() string
func (p *Plugin) Version() string
func (p *Plugin) Description() string
func (p *Plugin) State() State
func (p *Plugin) Path() string
func (p *Plugin) LoadedAt() time.Time
func (p *Plugin) Err() error
func (p *Plugin) Lookup(name string) (any, bool)
```

### Factory

```go
package factory

func NewManager(*config.Plugins) *ManagerBuilder

func (b *ManagerBuilder) UseLogger(*slog.Logger) *ManagerBuilder
func (b *ManagerBuilder) UseHealthCoordinator(*health.Coordinator) *ManagerBuilder
func (b *ManagerBuilder) UseHealthServiceName(string) *ManagerBuilder
func (b *ManagerBuilder) Build(context.Context) (*plugins.Manager, error)
```
