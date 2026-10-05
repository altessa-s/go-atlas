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

Known behavior: a successful listing that returns no secrets at all leaves the cache as it is. Cached secrets are neither evicted nor
cleared until a non-empty listing reconciles them, so a provider that transiently lists nothing cannot wipe the cache.

With a rebuildable negative filter the cycle also rebuilds the filter from the listing. When the filter is shared through Redis, only one
node rebuilds at a time: the others get `probfilter.ErrRebuildInProgress`, log it at debug level and still refresh their cache. Any other
rebuild failure, including `probfilter.ErrRebuildSuperseded`, is logged as an error; none fails the cycle.

## Value ownership

The `Manager` caches its own deep copies of what a provider hands out (payload and encoded bytes), and gives `Provider.Save` its own copy
of the payload. `Clear` — on eviction, `Delete`, `ClearCache` or shutdown — therefore only ever zeroes Manager-owned memory, never a
provider's: `providers/memory` keeps the values it returns, and the remote providers share results between concurrent callers.

| Accessor                       | Returns                                | Caller may modify / `Clear`  | Use when                       |
|--------------------------------|----------------------------------------|------------------------------|--------------------------------|
| `Value(ctx, key, force)`       | a deep copy owned by the caller        | yes — and should `Clear` it  | by default                     |
| `ValueShared(ctx, key, force)` | the `Manager`'s own instance, no copy  | no — read-only, never kept   | hot paths, read immediately    |

`ValueShared` saves the copy at a price: the `Manager` may clear its instance at any time — on eviction (capacity or update cycle),
`Delete`, `ClearCache` or shutdown — and a read that overlaps such a clear sees zeroed fields and is a data race. Use it only for keys that
are not concurrently deleted, evicted or cleared (for example a cache sized for all keys, without `ClearCache` while in use).

Copies are deep. A payload type with a `Clone() T` method controls its own copy; otherwise strings, slices, arrays, maps (keys and values),
pointers, interfaces and exported struct fields are copied by reflection, keeping identical references and cycles shared within the copy.
Overlapping slice views, interior pointers, unexported fields, channels and funcs are not reproduced by reflection — such payload types
should implement `Clone() T`.

Watch events carry their own copies too: `WatchEvent.Value` and `PreviousValue` belong to the receiver, which should `Clear` them when
done. The `Manager` never reads or clears them after sending, and clearing them affects neither the cache nor other watchers.

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
