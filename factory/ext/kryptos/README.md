# factory

```go
import "github.com/altessa-s/go-atlas/factory/ext/kryptos"
```

Package `factory` builds a [go-kryptos](https://github.com/altessa-s/go-kryptos) `manager.Manager` from the go-atlas secrets configuration.
`ManagerBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

The builder builds only the in-memory provider. A remote provider registers its `ProviderBuilder` with `UseProvider` from its own package; a
configured provider without one fails at `Build` naming the package to add.

## Quick Start

```go
manager, err := factory.New[any](cfg.Secrets).
    UseLogger(logger).
    UseProvider(secretsconfig.ProviderVault, vaultfactory.Builder[any](vaultClient)).
    UseScheduler(scheduler).
    UseHealthCoordinator(healthCoordinator).
    Build(ctx)
```

## Supported Providers

| Provider           | Config value | Register                                | Requires                                                    |
|--------------------|--------------|-----------------------------------------|-------------------------------------------------------------|
| Vault KV           | `vault`      | [`vaultfactory.Builder[any](client)`](vault) | A Vault client                                              |
| GCP Secret Manager | `gcp`        | [`gcpfactory.Builder[any]()`](gcp)           | The `gcp` section (project ID, service account path)        |
| Yandex Lockbox     | `lockbox`    | [`lockboxfactory.Builder[any]()`](lockbox)   | The `lockbox` section (folder ID, key ID, private key file) |
| In-memory          | `memory`     | built in                                | — (`memory.values` seeds it; tests and development)         |

A selected `gcp` or `lockbox` provider without its section fails with `ErrProviderConfigMissing`; a configuration loaded without its
`Validate` can produce one.

## Methods

### Constructor

| Method     | Description                                                        |
|------------|--------------------------------------------------------------------|
| `New[T](cfg)` | Creates a `ManagerBuilder` for the given secrets config            |

### Dependencies

| Method                 | Description                                                              |
|------------------------|--------------------------------------------------------------------------|
| `UseLogger`            | Sets the logger for the builder and all created components               |
| `UseDefaultLogger`     | Sets the logger to `slog.Default()`                                      |
| `UseProvider`          | Registers the `ProviderBuilder` of a remote provider type                |
| `UseScheduler`         | Registers `RunUpdateCycle` as the `secrets-cache-sync` task              |
| `UseHealthCoordinator` | Registers the manager's health as the `secrets` service                  |

### Terminal

| Method             | Description                                                                                                   |
|--------------------|---------------------------------------------------------------------------------------------------------------|
| `Build(ctx)`       | Assembles and returns the `*manager.Manager[T]`                                                               |

A manager built with `New[string]` decodes provider payloads with its value codec. The default codec reads JSON strings. Memory serves configured
strings directly. A `$__secret{namespace:key}` placeholder looks up the key `namespace:key`.

## Update cycle

go-kryptos starts no background loop. With `UseScheduler` and a non-empty `updateSchedule`, the builder registers the manager's
`RunUpdateCycle` as the `secrets-cache-sync` task (`runOnStart` honored). The memory provider is never reconciled, so it registers no task.
A rejected registration closes the manager and its provider. Closing a manager releases its scoped health checker.
The scheduler keeps the shared update task under `UpdateTaskID` so another manager can continue running it.
A scheduled callback for a closed manager returns without doing work. A scheduler passed to `UseScheduler` implements `task.Registrar`.

## Cache

| `maxSize` | `shardCount` | Result                                                     |
|-----------|--------------|------------------------------------------------------------|
| `0`       | `0`          | The manager defaults (`manager.DefaultCacheSize`, 1 shard) |
| `> 0`     | `0`          | One LRU shard of `maxSize` entries                         |
| any       | `> 0`        | `shardCount` shards; it must not exceed the cache size     |
