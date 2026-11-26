# redis

```go
import sagaredis "github.com/altessa-s/go-atlas/reliability/saga/storages/redis"
```

Durable [`saga.Storage`](../../storage.go) backed by Redis. Each saga instance is a hash holding the serialized payload and a `version`
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

`New` returns `(*Store, error)` and works with a standalone, sentinel or Redis Cluster client. The key prefix needs no hash tag: the store tags the keys
of each definition itself (see [Storage layout](#storage-layout)). An instance ID that is not valid UTF-8 is rejected with `ErrInvalidStorageKey`
(it wraps `ErrInvalidID`), because JSON would rewrite it.

## Recovery index

Redis is not query-capable, so recoverable instances are tracked in a sorted set scored by recover-eligibility time. `FetchRecoverable` reads the due
ids page by page after a cursor (`ZRANGEBYSCORE ... LIMIT` inside a script), so the transfer is bounded by the pages needed to fill the limit, and
counts an entry toward the limit only once it matches the requested definition and is recoverable (its lease is not active). An undecodable record is
skipped and logged by class. All writes keep the index consistent with the instance hash atomically
(inside the same Lua script):

| Instance state               | Index score      |
|------------------------------|------------------|
| holding an execution lease   | lease expiry (Unix ms, truncated) |
| `COMPENSATING`               | `0` (always due) |
| `RUNNING` with a deadline    | deadline (Unix)  |
| `RUNNING` without a deadline | not indexed      |
| terminal                     | removed          |

## Storage layout

| Key                                    | Type   | Contents                                |
|----------------------------------------|--------|-----------------------------------------|
| `<prefix>{d:<definition>}:i:<id>`      | Hash   | `d` = instance JSON, `v` = version      |
| `<prefix>{d:<definition>}:index:recoverable` | ZSet | member = instance ID, score = see above |
| `<prefix>{d:<definition>}:index:terminal`    | ZSet | member = instance ID, score = last update (Unix ms) |
| `<prefix>id:<id>`                      | String | name of the definition that owns the ID |
| `<prefix>definitions`                  | Set    | names of the definitions seen by `Create` |

The `{d:<definition>}` hash tag puts the instance hash and both indexes of one definition in one Redis Cluster slot, so every Lua script touches keys
of a single slot, and different definitions spread across slots. Braces in a definition name are percent-escaped. `Get` and `Delete` receive only an
ID, so they read `id:<id>` first; it also keeps IDs unique across definitions. `FetchRecoverable` and `DeleteTerminal` with an empty definition walk
the `definitions` set. Neither key is touched by a script. A lookup left by a crash reads as not found and is taken over by the next `Create`; with
`WithTTL` the lookup of a terminal instance expires with it. Create claims the ID in the lookup, writes the instance, then re-checks the claim and rolls back (returning `ErrInstanceExists`) when another definition
won it; `Delete` and retention remove the lookup before the instance. A key prefix containing a brace is rejected by `New`
(`ErrInvalidArgument`), and `Update` rejects a changed `Definition` (`ErrInvalidArgument`).

## Upgrade

Keys written by earlier versions (`<prefix>i:*`, `<prefix>index:*`) carry no definition tag and are ignored: instances in flight under them are neither
recovered nor listed. Finish or compensate in-flight sagas before upgrading, then delete the old keys. With `WithTTL` the lookup of a terminal instance
expires with it on a best-effort basis; a live instance's next write repairs an early expiry.

## Retention

Terminal instances are removed by the orchestrator's retention cycle (`saga.WithRetention`), which calls `DeleteTerminal` (paged over the terminal
index), or by `Store.Delete`. With the default `WithTTL(0)` they otherwise persist; a positive `WithTTL` makes a terminal instance expire on its
terminal write while a live instance never expires. (The NATS backend differs: its bucket carries a long backstop TTL by default.) The recoverable index scores
deadlines and lease expiries in Unix milliseconds, so a lease released within a second is recoverable at once. With a TTL every terminal write also
drops the terminal-index entries older than the TTL (their hashes have expired), so a TTL-only deployment's index stays bounded. Terminal instances
written before the terminal index existed are not in it and are never reaped by retention.

## See also

- [storages/memory](../memory) — in-process reference backend.
- [storages/nats](../nats) — durable NATS JetStream KeyValue backend.
- [storages/mongo](../mongo) — durable MongoDB backend.
