# secrets

```go
import "github.com/altessa-s/go-atlas/security/secrets"
```

Package `secrets` provides centralized secret management with automatic caching, real-time watch capabilities, and multiple storage backends.

## Key types

| Type / Function   | Description                                          |
|--------------------|------------------------------------------------------|
| `Manager[T]`      | Central orchestrator: caching, updates, lifecycle    |
| `Provider[T]`     | Interface for storage backends                       |
| `Value[T]`        | Secret container with metadata and secure cleanup    |
| `WatchResult[T]`  | Watch operation handle with event channel            |
| `WatchEvent[T]`   | Secret change event (Created, Updated, Deleted)      |
| `Locker`          | Distributed locking interface for write operations   |
| `Static`          | Marker interface for immutable secret sets           |

## Features

- LRU caching with configurable TTL and sharded locks
- Real-time watch API with event filtering
- Scheduler-based background cache refresh (`RunUpdateCycle`)
- Concurrent secret retrieval with worker pools
- Distributed locking for write operations
- Graceful shutdown with secure memory clearing

## Update cycle

`RunUpdateCycle` lists the storage, evicts (and clears) cached secrets missing from the listing, and caches new or changed versions. Keys
the `Manager` cached or deleted after the cycle began — `Save`, a forced `Value` fetch, `WarmCache`, `Delete` — are left alone, because the
listing may predate those writes: the cycle neither evicts, overwrites nor re-inserts them, and the next cycle reconciles them. A bounded
cache can still drop such an entry by capacity when the cycle inserts another key, as any insertion can; that never clears the value.

With a rebuildable negative filter the cycle also rebuilds the filter from the listing. When the filter is shared through Redis, only one
node rebuilds at a time: the others get `probfilter.ErrRebuildInProgress`, log it at debug level and still refresh their cache. Any other
rebuild failure, including `probfilter.ErrRebuildSuperseded`, is logged as an error; none fails the cycle.

## Security

Secret material never reaches the log. Debug-level records from the `Manager` carry the secret's **key** — the lookup identifier validated
against `[a-zA-Z0-9_.-]+`, not the value behind it — so cache miss, fetch, save and delete records for one secret can be correlated.

The payload itself is protected on two levels: `Value.Value` and `Value.EncodedValue` carry `json:"-"`, so an accidental `json.Marshal`
emits metadata only, and `Value.LogValue` implements `slog.LogValuer` to report the same metadata when a `Value` reaches a logger. slog
resolves a `LogValuer` atomically, so the secret fields cannot leak through field expansion either.

Key names are internal identifiers, but a naming scheme can itself disclose infrastructure layout. When that matters, run production
loggers at Info or above, or wrap the handler with
[`observability/slog/handler/masking`](../../observability/slog/handler/masking/README.md).

## Subpackages

| Package                                | Description                           |
|----------------------------------------|---------------------------------------|
| [codec](./codec)                       | Key/value encoding interfaces         |
| [factory](./factory)                   | Config-based manager creation         |
| [providers/gcp](./providers/gcp)       | Google Cloud Secret Manager backend   |
| [providers/lockbox](./providers/lockbox) | Yandex Cloud Lockbox backend        |
| [providers/memory](./providers/memory) | In-memory backend for testing         |
| [providers/vault](./providers/vault)   | HashiCorp Vault KV v2 backend         |
