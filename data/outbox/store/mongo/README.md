# mongo

```go
import "github.com/altessa-s/go-atlas/data/outbox/store/mongo"
```

Package `mongo` implements `outbox.Store` using MongoDB as the backing store. Provides durable event persistence with indexed queries for
efficient batch fetch, status updates, and cleanup.

## Server clock

All time-based predicates (retry-after, lock-expiry, retention, expiry) are evaluated against the **MongoDB server clock** via the `$$NOW` aggregation
variable inside `$expr` filters and pipeline updates — never the application's wall clock. The timestamps those predicates compare against
(`last_attempt_on`, `next_attempt_at`, `published_at`, `locked_on`) are written from the same clock, so both sides of every comparison come from one
source; a client-stamped field would reintroduce the skew the predicates avoid. A worker with a skewed clock therefore cannot prematurely
unlock another worker's in-flight event or leak a stuck one: every comparison uses the single authoritative clock shared by all workers. Timestamps are
stored as BSON `Date` (required for `$$NOW` arithmetic), with absent optional timestamps represented as missing fields rather than the Unix epoch.

## Lock fencing

Locking an event stamps it with a `lock_token` minted per fetch, and every `UpdateEvents` write filters on that token. A dispatcher whose lease the
unlock sweeper revoked therefore cannot write its result: the token no longer matches and the stale write is dropped instead of overwriting the
outcome produced by whichever worker picked the event up afterwards. Unlocking and expiring both clear the token, which is what ends the lease.

## Retry backoff

A failed event carries `next_attempt_at`, computed server-side as `$$NOW` plus the `Event.RetryAfter` duration the outbox supplied. Only a duration
crosses the process boundary, so the deadline stays anchored to the database clock. `FetchUnprocessedEvents` treats an absent `next_attempt_at` as
"eligible now", which is also how documents written before this field existed behave — no backfill is required.

## Change streams

`Store` implements the optional `outbox.Watcher` interface with a change stream over the outbox collection, so `Outbox.Watch` dispatches a saved
event immediately instead of waiting for the next poll tick. Inserts made inside a transaction surface when that transaction commits — exactly
when the event became real.

Change streams are built on the oplog, so they need a replica set (a single-node one counts) or a sharded cluster. `SupportsChangeStreams`
probes the topology with `hello` (falling back to `isMaster` on servers older than 4.4.2) and `Watch` returns `outbox.ErrWatchUnsupported`
without opening a stream when the deployment is a standalone `mongod`.

| Aspect                | Behavior                                                                                          |
|-----------------------|---------------------------------------------------------------------------------------------------|
| Matched operations    | `insert` only — retries and lock expiry are time-driven and produce no change event                |
| Payload               | Projected down to `_id` (the resume token); an insert event would otherwise carry the full payload |
| Transient failures    | Resumed by the driver from its own resume token                                                     |
| Terminal failures     | Reopened from the last resume token after exponential backoff with jitter (1s → 30s)                |
| Resume token expired  | Stream restarted from now, plus one synthetic notification — the gap is unrecoverable from a stream |
| Collection dropped    | Token discarded; the stream reopens from now                                                        |

The watcher is a latency optimization layered on the dispatch cycle, not a replacement for it. See [../..](../..#change-notifications).

## Fields

| Field             | Purpose                                                        |
|-------------------|----------------------------------------------------------------|
| `status`          | Event lifecycle state                                          |
| `created_at`      | Creation time; also the fetch sort key                         |
| `last_attempt_on` | Server-stamped time of the most recent attempt                 |
| `next_attempt_at` | Server-stamped retry deadline; absent means eligible now       |
| `locked_on`       | Server-stamped lock time; absent when unlocked                 |
| `lock_token`      | Fencing token for the current lease                            |
| `published_at`    | Server-stamped completion time, used by the retention sweep    |
| `expires_at`      | Dispatch deadline; absent means no expiration                  |
| `error`           | Last dispatch error, retained for dead-letter inspection       |

## Migration

Legacy collections stored timestamps as int64 Unix seconds. Run the in-place, idempotent migration **once per collection** before serving traffic with
this store, then wire it into your migration registry (do not call it from `New`):

| Function                                       | Direction                          |
|------------------------------------------------|------------------------------------|
| `MigrateTimestampsToDate(ctx, collection)`     | int64 Unix seconds → BSON `Date`   |
| `RevertTimestampsToUnix(ctx, collection)`      | BSON `Date` → int64 Unix seconds (rollback) |

Both return the number of documents converted, only touch documents still in the source layout (so re-running is a no-op), and drop optional
timestamps that are zero/absent rather than mapping them to `1970`. The rollback also removes `lock_token` and `next_attempt_at`, which the legacy
layout has no notion of — a stale lock token left behind would fence out an event's own writes after a roll forward.

`lock_token` and `next_attempt_at` need no forward migration: an absent `next_attempt_at` means "eligible now", so documents written before these
fields existed are picked up unchanged.
