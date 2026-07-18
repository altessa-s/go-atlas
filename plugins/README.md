# plugins

```go
import "github.com/altessa-s/go-atlas/plugins"
```

Dynamic plugin manager for loading and managing `.so` plugins at runtime,
inspired by Keycloak's SPI (Service Provider Interface). Framework-agnostic:
manages plugin lifecycle while consuming services define their own provider
interfaces and discover them via symbol lookup.

> **⚠️ SECURITY: Signature verification is enabled by default.** `NewManager()` rejects unsigned plugins unless verification is explicitly
> disabled via `WithSignatureDisabled` (development and tests only). See [Security](#security) below.

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

- `HostVersion: "2.1.0"` — host service semver this plugin was built
  against. When the manager is configured with `WithHostVersion` (the
  factory package auto-wires this from `appinfo.Version`), majors are
  compared at load time. Mismatches return `ErrHostVersionMismatch`
  unless `WithHostVersionMode` is set to `HostVersionWarn` /
  `HostVersionDisabled`.

Optional symbols:

- `func Init(ctx context.Context) error` or `var Init = func(ctx context.Context) error { … }`
- `var DepInfo = plugins.NewDepInfoFromBuild()`
- `var <Symbol>SPIVersion = plugins.SPIVersion{Contract: "…", Major: N, Minor: N}`

## Usage

```go
mgr := plugins.NewManager(
    plugins.WithDir("./plugins"),
    plugins.WithInitTimeout(10*time.Second),
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
| `WithLogger`             | default slog         | Structured logger                                                                    |
| `WithMetrics`            | unset (no metrics)   | Collector for load, signature, quarantine, and Init metrics                          |
| `WithHostVersion`        | unset (check off)    | Host service semver compared against `Descriptor.HostVersion` major; factory auto-wires from `appinfo.Version` |
| `WithHostVersionMode`    | `HostVersionEnforce` | Strictness when both versions declared: `Enforce` (reject), `Warn` (log), `Disabled` |
| `WithSandbox`            | disabled             | Linux process-hardening primitives                                                   |
| `WithSignature`          | `SignatureRequire`   | Cryptographic `.so.sig` verification (default: require signature)                    |
| `WithSignatureDisabled`  | —                    | Explicit opt-out from signature verification; development and tests only             |
| `WithMultiKeySignature`  | —                    | Multiple accepted public keys for zero-downtime signing-key rotation                 |

## Security

Loading a plugin executes arbitrary native code in the host process: `plugin.Open` runs the plugin's `init` functions before the manager can
inspect anything, and Go cannot unload a plugin. Treat every `.so` under the configured directory as fully trusted code from the same supply
chain as the host binary:

- Restrict the plugin directory to a path writable only by the deployer (not the application user); in production prefer a read-only mount.
- Never load plugins from user uploads, network shares, or world-writable locations.
- Keep signature verification in its default `SignatureRequire` mode.
- Sign `.so` files with the [`plugin-sign`](../cmd/plugin-sign/README.md) CLI (Ed25519 recommended; ECDSA P-256 and RSA-PSS also supported);
  rotate signing keys without downtime via `WithMultiKeySignature`.

With signatures enabled the manager verifies the detached `.sig` before `plugin.Open` and re-hashes the file after it, quarantining the plugin
on mismatch (`ErrPluginModified`). This narrows — but cannot fully close — the TOCTOU window of Go's path-based plugin API; filesystem
permissions remain the primary control. See the Security section in the [package documentation](doc.go) for the full threat model.

## Platform support

Plugin loading is supported on `darwin` and `linux` only (Go `plugin` package
limitation). On other platforms `Manager.Load` returns `ErrUnsupportedPlatform`.

See [docs/plugins.md](../docs/plugins.md) for the full reference.
