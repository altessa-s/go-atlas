# mongo

```go
import "github.com/altessa-s/go-atlas/reliability/outbox/storages/mongo"
```

Package `mongo` implements `outbox.Storage` using MongoDB as the backing store. Provides durable event persistence with indexed queries for efficient
batch fetch, status updates, and cleanup.

## Server clock

All time-based predicates (retry-after, lock-expiry, retention, expiry) are evaluated against the **MongoDB server clock** via the `$$NOW` aggregation
variable inside `$expr` filters and pipeline updates — never the application's wall clock. The timestamps those predicates compare against
(`last_attempt_on`, `next_attempt_at`, `published_at`, `locked_on`) are written from the same clock, so both sides of every comparison come from one
source; a client-stamped field would reintroduce the skew the predicates avoid. A worker with a skewed clock therefore cannot prematurely unlock another
worker's in-flight event or leak a stuck one: every comparison uses the single authoritative clock shared by all workers. Timestamps are stored as BSON
`Date` (required for `$$NOW` arithmetic), with absent optional timestamps represented as missing or `null` fields rather than the Unix epoch.

## Lock fencing

Locking an event stamps it with a `lock_token` minted per fetch, and every `UpdateEvents` write filters on that token. A dispatcher whose lease the
unlock sweeper revoked therefore cannot write its result: the token no longer matches and the stale write is dropped instead of overwriting the outcome
produced by whichever worker picked the event up afterwards. Unlocking and expiring both clear the token, which is what ends the lease.

## Retry backoff

A failed event carries `next_attempt_at`, computed server-side as `$$NOW` plus the `Event.RetryAfter` duration the outbox supplied. Only a duration
crosses the process boundary, so the deadline stays anchored to the database clock. `FetchUnprocessedEvents` treats an absent `next_attempt_at` as
"eligible now".

## Change streams

`Store` implements the optional `outbox.Watcher` interface with a change stream over the outbox collection, so `Outbox.Watch` dispatches a saved event
immediately instead of waiting for the next poll tick. Inserts made inside a transaction surface when that transaction commits — exactly when the event
became real.

Change streams are built on the oplog, so they need a replica set (a single-node one counts) or a sharded cluster. `SupportsChangeStreams` probes the
topology with `hello` (falling back to `isMaster` on servers older than 4.4.2) and `Watch` returns `outbox.ErrWatchUnsupported` without opening a stream
when the deployment is a standalone `mongod`.

| Aspect                | Behavior                                                                                          |
|-----------------------|---------------------------------------------------------------------------------------------------|
| Matched operations    | `insert` only — retries and lock expiry are time-driven and produce no change event                |
| Payload               | Projected down to `_id` (the resume token); an insert event would otherwise carry the full payload |
| Transient failures    | Resumed by the driver from its own resume token                                                     |
| Terminal failures     | Reopened from the last resume token after exponential backoff with jitter (1s → 30s)                |
| Resume token expired  | Stream restarted from now, plus one synthetic notification — the gap is unrecoverable from a stream |
| Collection dropped    | Token discarded; the stream reopens from now                                                        |
| Probe fails at start  | Retried with the same backoff, 10s per attempt, until the deployment answers or the context ends    |

The watcher is a latency optimization layered on the dispatch cycle, not a replacement for it. See [../..](../..#change-notifications).

Failed probes and stream reconnects are reported on the `WithLogger` logger (discarded by default): a warning on the first failure of a run and then
on every tenth, which at the 30s backoff cap is one record every five minutes, plus one info record when the probe answers after failures. The records
carry the driver's error, never the connection string — the store holds only a collection handle.

## Fields

| Field             | Purpose                                                        |
|-------------------|----------------------------------------------------------------|
| `status`          | Event lifecycle state                                          |
| `created_at`      | Creation time; also the fetch sort key                         |
| `seq`             | Insertion-order tie-breaker for events sharing a `created_at`  |
| `last_attempt_on` | Server-stamped time of the most recent attempt                 |
| `next_attempt_at` | Server-stamped retry deadline; absent means eligible now       |
| `locked_on`       | Server-stamped lock time; absent when unlocked                 |
| `lock_token`      | Fencing token for the current lease                            |
| `published_at`    | Server-stamped completion time, used by the retention sweep    |
| `expires_at`      | Dispatch deadline; absent means no expiration                  |
| `error`           | Last dispatch error, retained for dead-letter inspection       |

## Indexes

The constructor creates the indexes below; existing ones are left as they are.

| Index                          | Serves                                                         |
|--------------------------------|----------------------------------------------------------------|
| `status, created_at, seq`      | The fetch: one query per ready status, read in fetch order     |
| `status, locked_on`            | The unlock sweep                                               |
| `status, published_at`         | The retention sweep                                            |
| `status, expires_at` (partial) | The expiry sweep; holds only events with a TTL                 |

`FetchUnprocessedEvents` queries pending events and failed events separately, each an equality on `status` read through the first index in
`created_at, seq` order, and merges the two batches. A single `$or` over both statuses cannot take its sort from one index, so the server would sort the
whole backlog for every batch.

The index cannot bound retry eligibility: the failed query walks failed events in creation order and skips those whose `next_attempt_at` is still
ahead, so a large failed backlog that is all backing off is read on every fetch. The SQL store reads failed rows the same way. Keep the failed backlog
small with `outbox.WithRetryMaxAttempts` rather than long backoffs.
