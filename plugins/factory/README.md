# factory

```go
import "github.com/altessa-s/go-atlas/plugins/factory"
```

Package `factory` provides a fluent builder for creating plugin managers from configuration. `ManagerBuilder.Build(ctx)` checks the configuration,
loads plugins, and optionally starts watching, returning on the first failing stage.

## Quick Start

```go
manager, err := factory.NewManager(cfg.Plugins).
    UseLogger(logger).
    UseHealthCoordinator(healthCoordinator).
    Build(ctx)
```

With custom health service name:

```go
manager, err := factory.NewManager(cfg.Plugins).
    UseLogger(logger).
    UseHealthCoordinator(healthCoordinator).
    UseHealthServiceName("custom-plugins").
    Build(ctx)
```

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `NewManager(cfg)` | Creates a `ManagerBuilder` for the given plugins config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the manager |
| `UseHealthCoordinator` | Registers the manager with a health coordinator |
| `UseHealthServiceName` | Overrides the health check service name (default: "plugins") |

### Terminal

| Method | Description |
|--------|-------------|
| `Build(ctx)` | Assembles the manager, loads plugins, starts the watcher when `watch` is set; fails when the config is nil or disabled |

## Configuration

The builder consumes a [`pluginsconfig.Config`](../../config/plugins/plugins.go) struct which controls:

- Plugin directory path and snapshot directory
- Load allowlist, disabled list, Init timeout, and directory watching
- Signature verification settings
- Sandbox settings (mapped by `SandboxOptionsFromConfig`)

The host version is not configured here: the builder auto-wires `plugins.WithHostVersion` from `appinfo.Version` when it is stamped (not empty and not
`0.0.0`).

## Health Integration

When a health coordinator is provided, the manager is registered as a health check service. The check reports `StatusDegraded` when some plugins failed
or a watcher reload failed, and `StatusNotServing` when all plugins failed, the sandbox failed, or the manager is closed. The `snapshotDir`
configuration is forwarded to `plugins.WithSnapshotDir`; select a trusted executable mount when the OS temp mount is `noexec`.
