# plugins

```go
import "github.com/altessa-s/go-atlas/plugins"
```

Dynamic plugin manager for loading and managing `.so` plugins at runtime, inspired by Keycloak's SPI (Service Provider Interface). Framework-agnostic:
manages plugin lifecycle while consuming services define their own provider interfaces and discover them via symbol lookup.

> **⚠️ SECURITY: Signature verification is enabled by default.** `NewManager()` rejects unsigned plugins unless the mode is `SignatureWarn` (missing
> signatures load with a warning) or verification is explicitly disabled via `WithSignatureDisabled` (development and tests only). See
> [Security](#security) below.

## Key types

| Type             | Purpose                                               |
|------------------|-------------------------------------------------------|
| `Manager`        | Loads, discovers, and manages plugin lifecycle         |
| `Plugin`         | Represents a loaded `.so` with state and symbol cache  |
| `Descriptor`     | Static metadata every plugin must export               |
| `DepInfo`        | Optional build-dependency snapshot for skew detection  |
| `SPIVersion`     | Contract version a plugin provider implements          |
| `SPIConstraint`  | Host-side version requirements for SPI negotiation     |

## Plugin contract

Every `.so` must export:

- `var Descriptor = plugins.Descriptor{Name: "…", Version: "…", GoVersion: runtime.Version()}`

Optional `Descriptor` field:

- `HostVersion: "2.1.0"` — host service semver this plugin was built against. When the manager is configured with `WithHostVersion` (the factory package
  auto-wires this from `appinfo.Version`), majors are compared at load time. Mismatches return `ErrHostVersionMismatch` unless `WithHostVersionMode` is
  set to `HostVersionWarn` / `HostVersionDisabled`.

Optional symbols:

- `func Init(ctx context.Context) error` or `var Init = func(ctx context.Context) error { … }`
- `var DepInfo = plugins.NewDepInfoFromBuild()`
- `var <Symbol>SPIVersion = plugins.SPIVersion{Contract: "…", Major: N, Minor: N}`

## Usage

```go
mgr := plugins.NewManager(
    plugins.WithDir("./plugins"),
    plugins.WithInitTimeout(10*time.Second),
    plugins.WithSignature(plugins.SignatureOptions{
        Mode:          plugins.SignatureRequire,
        PublicKeyPath: "/etc/keys/plugin-public.pem",
    }),
)
defer mgr.Close()

if err := mgr.Load(ctx); err != nil {
    slog.Warn("some plugins failed", slog.Any("error", err))
}

// SPI discovery.
for p, sym := range mgr.LookupAll("AuthProvider") {
    provider, ok := sym.(auth.Provider)
    if !ok { continue }
    // use provider
}

// SPI discovery with version negotiation.
constraint := plugins.SPIConstraint{Major: 2, MinMinor: 0}
for p, sym := range plugins.NegotiateAll(mgr, "AuthProvider", constraint, logger) {
    provider, ok := sym.(auth.Provider)
    if !ok { continue }
    // provider is version-compatible
}

// Quarantine a misbehaving plugin at runtime.
if err := mgr.Quarantine("broken-plugin"); err != nil {
    slog.Error("quarantine", slog.Any("error", err))
}
```

## Options

| Option                   | Default              | Description                                                                          |
|--------------------------|----------------------|--------------------------------------------------------------------------------------|
| `WithDir`                | `./plugins`          | Directory scanned for `.so` files                                                    |
| `WithLoad`               | `[]`                 | Allowlist of filenames                                                               |
| `WithDisabled`           | `[]`                 | Exclusion list                                                                       |
| `WithInitTimeout`        | `5s`                 | Per-plugin Init timeout                                                              |
| `WithWatchDebounce`      | `200ms`              | Filesystem event coalesce window                                                     |
| `WithLogger`             | discard              | Structured logger (default: `slog.DiscardHandler`)                                   |
| `WithMetrics`            | unset (no metrics)   | Collector for load, signature, quarantine, and Init metrics                          |
| `WithHostVersion`        | unset (check off)    | Host service semver compared against `Descriptor.HostVersion` major; factory auto-wires from `appinfo.Version` |
| `WithHostVersionMode`    | `HostVersionEnforce` | Strictness when both versions declared: `Enforce` (reject), `Warn` (log), `Disabled` |
| `WithSandbox`            | disabled             | Linux process-hardening primitives                                                   |
| `WithSnapshotDir`        | OS temp directory    | Trusted executable base for private plugin copies                                  |
| `WithSignature`          | `SignatureRequire`   | Cryptographic `.so.sig` verification (default: require signature)                    |
| `WithSignatureDisabled`  | —                    | Explicit opt-out from signature verification; development and tests only             |
| `WithMultiKeySignature`  | —                    | Multiple accepted public keys for zero-downtime signing-key rotation                 |

## Security

Loading a plugin executes arbitrary native code in the host process: `plugin.Open` runs the plugin's `init` functions before the manager can inspect
anything, and Go cannot unload a plugin. Treat every `.so` under the configured directory as fully trusted code from the same supply chain as the host
binary:

- Restrict the plugin directory to a path writable only by the deployer (not the application user); in production prefer a read-only mount.
- Never load plugins from user uploads, network shares, or world-writable locations.
- Keep signature verification in its default `SignatureRequire` mode.
- Sign `.so` files with the [`plugin-sign`](../cmd/plugin-sign/README.md) CLI (Ed25519 recommended; ECDSA P-256 and RSA-PSS also supported); rotate
  signing keys without downtime via `WithMultiKeySignature`.

The manager loads a private copy of the exact bytes checked by signature verification, preventing a source-file swap from changing the executed code.
Snapshot hash mismatches fail before opening (`ErrPluginModified`). Files use mode 0600 inside a fresh 0700 directory; they are unlinked after opening.
`Close` removes the directory once any in-flight open finishes. Successful handles are cached by content hash across managers; every load still verifies
its signature. Runtime quarantine records the loaded hash, even if the source has changed.

`WithSnapshotDir` (config: `snapshotDir`) selects a trusted executable base directory; empty uses `os.TempDir`. Set it when the default temp mount is
`noexec`. Group/other-writable bases require the sticky bit. Protect base ownership, ancestors and ACLs from the plugin producer; these permissions
cannot isolate code running under the host UID or root. Landlock automatically permits read/write/execute access to the private directory. See
[package documentation](doc.go) for the full threat model.

## Platform support

Plugin loading is supported on `darwin` and `linux` only (Go `plugin` package limitation). On other platforms `Manager.Load` returns
`ErrUnsupportedPlatform`.

See [docs/plugins.md](../docs/plugins.md) for the full reference.

### Snapshot runtime constraints

Landlock only grants access inside the snapshot directory. Removing that directory also needs permission on its parent: configure a dedicated trusted
`snapshotDir` in `Landlock.ReadWritePaths` for full cleanup, or arrange external cleanup of empty private directories. `Close` returns removal errors
and subsequent calls retry; errors deferred until an active open finishes are logged.

Go cannot replace a loaded plugin identity with changed bytes; restart the host for upgrades. Plugins opened outside this manager cache are not
supported. Native dependencies using `$ORIGIN`/`@loader_path` no longer resolve beside the original `.so`; use trusted absolute library paths.
Dependency bytes are not authenticated by the plugin signature. Plugin initialization must not recursively load the same plugin, or call
`Close`/`StopWatching` during a watcher reload (those methods wait for the watcher).

### Linux loader integration test

On Linux with cgo, a C compiler and Landlock enabled in the kernel/container policy, run:

```sh
ATLAS_PLUGIN_INTEGRATION=1 go test ./plugins -run '^TestSnapshotLandlockIntegration$' -count=1 -v
```

Run without `-race`: this test builds a real shared object and applies irreversible Landlock rules in child processes pinned to an OS thread. It checks
signed snapshot execution, handle reuse, file unlinking, and directory cleanup with and without permission on the parent. Landlock child cases
explicitly skip when the kernel reports that the feature is unsupported; the real loader case still runs. The ordinary suite separately checks
concurrent cache and cleanup behavior with the race detector.
