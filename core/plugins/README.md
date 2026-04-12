# plugins

```go
import "github.com/altessa-s/go-atlas/core/plugins"
```

Dynamic plugin manager for loading and managing `.so` plugins at runtime,
inspired by Keycloak's SPI (Service Provider Interface). Framework-agnostic:
manages plugin lifecycle while consuming services define their own provider
interfaces and discover them via symbol lookup.

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
```

## Options

| Option              | Default      | Description                           |
|---------------------|--------------|---------------------------------------|
| `WithDir`           | `./plugins`  | Directory scanned for `.so` files     |
| `WithLoad`          | `[]`         | Allowlist of filenames                |
| `WithDisabled`      | `[]`         | Exclusion list                        |
| `WithInitTimeout`   | `5s`         | Per-plugin Init timeout               |
| `WithWatchDebounce` | `200ms`      | Filesystem event coalesce window      |
| `WithLogger`        | default slog | Structured logger                     |
| `WithSandbox`       | disabled     | Linux process-hardening primitives    |

## Platform support

Plugin loading is supported on `darwin` and `linux` only (Go `plugin` package
limitation). On other platforms `Manager.Load` returns `ErrUnsupportedPlatform`.

See [docs/plugins.md](../../docs/plugins.md) for the full reference.
