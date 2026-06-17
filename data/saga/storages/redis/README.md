# redis

```go
import sagaredis "github.com/altessa-s/go-atlas/data/saga/storages/redis"
```

Durable [`saga.Store`](../../store.go) backed by Redis. Each saga instance is a hash keyed by its ID holding the serialized payload and a `version`
field used as the optimistic-concurrency token: `Update` is a Lua compare-and-set on `version`, so a stale writer is rejected with
`errs.ErrVersionConflict` and concurrent recovery cycles cannot double-advance an instance.

## Constructor

| Function           | Description                                                    |
|--------------------|---------------------------------------------------------------|
| `New(client, ...)` | Store on a `redis.UniversalClient`. Panics if `client` is nil. |

## Options

| Option           | Default  | Description                                                       |
|------------------|----------|-------------------------------------------------------------------|
| `WithKeyPrefix`  | `saga:`  | Prefix applied to all keys.                                       |
| `WithTTL`        | 0        | Per-key expiry applied on every write; `0` persists indefinitely. |

## Recovery index

Redis is not query-capable, so recoverable instances are tracked in a sorted set scored by recover-eligibility time. `FetchRecoverable` is a single
`ZRANGEBYSCORE` from `-inf` to the current time. All writes keep the index consistent with the instance hash atomically (inside the same Lua script):

| Instance state               | Index score      |
|------------------------------|------------------|
| `COMPENSATING`               | `0` (always due) |
| `RUNNING` with a deadline    | deadline (Unix)  |
| `RUNNING` without a deadline | not indexed      |
| terminal                     | removed          |

## Storage layout

| Key                         | Type | Contents                                |
|-----------------------------|------|-----------------------------------------|
| `<prefix>i:<id>`            | Hash | `d` = instance JSON, `v` = version      |
| `<prefix>index:recoverable` | ZSet | member = instance ID, score = see above |

## Retention

Terminal instances are **not** deleted automatically — the orchestrator keeps them for inspection and leaves disposal to the caller. With the default
`WithTTL(0)` (persist forever) completed and failed instances accumulate until you call `Store.Delete`; set a positive `WithTTL` to have every write
apply a `PEXPIRE` so instances self-expire. (The NATS backend differs: its bucket carries a long backstop TTL by default.) The recoverable index scores
deadlines in Unix seconds, so recovery eligibility is evaluated at one-second granularity.

## See also

- [storages/memory](../memory) — in-process reference backend.
- [storages/nats](../nats) — durable NATS JetStream KeyValue backend.
- [storages/mongo](../mongo) — durable MongoDB backend.
