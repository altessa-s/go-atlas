# redis

```go
import sagaredis "github.com/altessa-s/go-atlas/reliability/saga/storages/redis"
```

Durable [`saga.Storage`](../../storage.go) backed by Redis. Each saga instance is a hash keyed by its ID holding the serialized payload and a `version`
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
| `WithTTL`        | 0        | Expiry of a terminal instance, set on its terminal write; a live instance never expires. `0` persists indefinitely. |
| `WithLogger`     | discard  | Logger for skipped undecodable records (class and size only).     |

`New` returns `(*Store, error)`. On Redis Cluster the key prefix must contain a hash tag (`WithKeyPrefix("{saga}:")`) so an instance and its indexes
share a slot; a cluster client with an untagged prefix fails with an error matching `ErrInvalidArgument`. An instance ID that is not valid UTF-8 is
rejected with `ErrInvalidStorageKey` (it wraps `ErrInvalidID`), because JSON would rewrite it.

## Recovery index

Redis is not query-capable, so recoverable instances are tracked in a sorted set scored by recover-eligibility time. `FetchRecoverable` reads the due
ids page by page after a cursor (`ZRANGEBYSCORE ... LIMIT` inside a script), so the transfer is bounded by the pages needed to fill the limit, and
counts an entry toward the limit only once it matches the requested definition and is recoverable (its lease is not active). An undecodable record is
skipped and logged by class. All writes keep the index consistent with the instance hash atomically
(inside the same Lua script):

| Instance state               | Index score      |
|------------------------------|------------------|
| holding an execution lease   | lease expiry (Unix) + 1 |
| `COMPENSATING`               | `0` (always due) |
| `RUNNING` with a deadline    | deadline (Unix)  |
| `RUNNING` without a deadline | not indexed      |
| terminal                     | removed          |

## Storage layout

| Key                         | Type | Contents                                |
|-----------------------------|------|-----------------------------------------|
| `<prefix>i:<id>`            | Hash | `d` = instance JSON, `v` = version      |
| `<prefix>index:recoverable` | ZSet | member = instance ID, score = see above |
| `<prefix>index:terminal`    | ZSet | member = instance ID, score = last update (Unix ms) |

## Retention

Terminal instances are removed by the orchestrator's retention cycle (`saga.WithRetention`), which calls `DeleteTerminal` (paged over the terminal
index), or by `Store.Delete`. With the default `WithTTL(0)` they otherwise persist; a positive `WithTTL` makes a terminal instance expire on its
terminal write while a live instance never expires. (The NATS backend differs: its bucket carries a long backstop TTL by default.) The recoverable index scores
deadlines in Unix seconds, so recovery eligibility is evaluated at one-second granularity.

## See also

- [storages/memory](../memory) — in-process reference backend.
- [storages/nats](../nats) — durable NATS JetStream KeyValue backend.
- [storages/mongo](../mongo) — durable MongoDB backend.
