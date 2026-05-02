# storages

```go
import "github.com/altessa-s/go-atlas/data/idempotency/storages"
```

Package `storages` defines the `Storage` interface for idempotency key persistence. Implementations live in subpackages and are injected into
the top-level `idempotency.Keeper` to supply the underlying storage backend.

## Key types

| Type / Interface | Description                                                    |
|------------------|----------------------------------------------------------------|
| `Storage`        | Interface: AttemptLock, AttemptLockWithTTL, Complete, Steal, Delete |
| `Status`         | Key state: `StatusInProgress`, `StatusSuccess`                 |
| `State`          | Holds the current status and optional response data for a key  |

`Steal` is a low-level CAS-replace primitive used by Keeper's
orphan-lock reclaim path: when a collision-time check spots an
abandoned InProgress entry older than the configured threshold,
Keeper builds a fresh wire and atomically swaps it in via
`Steal(ctx, key, expectedVal, newVal)`. Backends implement it on top
of their existing CAS mechanism (mutex+`bytes.Equal` for memory, Lua
GET-compare-SET for Redis, `kv.Update(... revision)` for NATS).

## Subpackages

| Package                | Description                        |
|------------------------|------------------------------------|
| [memory](./memory)     | In-memory backend with TTL         |
| [nats](./nats)         | NATS JetStream storage             |
| [redis](./redis)       | Distributed Redis storage          |
