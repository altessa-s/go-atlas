# nats

```go
import "github.com/altessa-s/go-atlas/coord/leadelect/providers/nats"
```

Package `nats` implements leader election using NATS JetStream key-value store with TTL-based lease renewal. Provides distributed leader election
without external dependencies beyond NATS.

## Constructors

| Constructor                                  | Description                                                                          |
|----------------------------------------------|--------------------------------------------------------------------------------------|
| `New(ctx, conn, opts...)`                    | The election `Provider`, tuned by the options below                                  |
| `NewLeader(ctx, conn, key, nodeID, opts...)` | A `leadelect.Leader` over a default `Provider`; `opts` are `leadelect.Option` values |

## Options

| Option                 | Default                   | Description                                                                                |
|------------------------|---------------------------|--------------------------------------------------------------------------------------------|
| `WithBucket`           | `leadelect`               | KeyValue bucket name for the election key.                                                 |
| `WithRenewRatio`       | `1/3`                     | Fraction of the lease lifetime at which the lease is renewed.                              |
| `WithStorage`          | `jetstream.MemoryStorage` | JetStream storage backend for the bucket. Use `jetstream.FileStorage` to survive restarts. |
| `WithMigrateBucketTTL` | off                       | Update an existing bucket whose key TTL is not `DefaultBucketKeysTTL` instead of failing.  |
| `WithMeter`            | no-op                     | Metrics collector.                                                                         |
| `WithLogger`           | discard                   | Structured logger.                                                                         |

Memory storage keeps the lease ephemeral (lost on a JetStream restart, forcing a clean re-election) and avoids disk I/O on the renew hot path. Choose
`WithStorage(jetstream.FileStorage)` when the bucket — and the monotonic fencing revision behind `Fence()` — must survive a full server bounce.

Through the YAML factory the storage is `leaderElector.storage: memory|file` (default `memory`). The server cannot change a bucket's storage type, so
`New` returns `ErrBucketStorageMismatch` and leaves the bucket untouched when it already exists with another one; align the configuration, recreate the
bucket, or use another bucket.

## Keys

NATS KeyValue accepts only `[-/_=.a-zA-Z0-9]` in a key, with no leading or trailing dot and no empty token, so election keys are escaped before they
reach the bucket: every other byte, `=` itself, and every dot that would leave an empty token become `=HH` (upper-case hex). The election key
`scheduler:eu` is stored as `scheduler=3Aeu`; `Start` rejects an oversized key before it starts campaigning. A key that is already valid and contains
no `=` is stored under itself. The escaped key may be at most 3072 bytes (an escaped byte takes three); a longer one fails with `ErrKeyTooLong`
before any network I/O.

## Lease renewal

Renewals tick every `min(electionTTL, bucketKeyTTL) × renewRatio`. The ratio therefore decides how many renewal attempts fall inside one lease
lifetime — `floor(1/ratio)` of them — and that count is the budget for surviving a transient NATS failure without a spurious re-election. At `0.75`
the
budget is a single attempt, so one dropped packet costs leadership; at `1/3` three ticks land inside the window, two of them with room to spare. One
third is the usual choice for lease keepalives (etcd sessions and ZooKeeper heartbeats both renew at `TTL/3`).

A ratio outside `(0, 1]` falls back to the default: a non-positive interval would make `time.NewTicker` panic inside the camping goroutine, where the
recover would swallow it and leave an elector that reports itself running while never electing anyone.

## Bucket TTL

The bucket's key TTL is the expiry mechanism behind the lease: it is what releases the election key when a holder dies without resigning. A
pre-existing
bucket with a different key TTL — or none, which would yield leases that never expire and hang the election until someone intervenes by hand — is
therefore refused: `New` returns `ErrBucketTTLMismatch` and leaves the bucket untouched, since rewriting it would change the key lifetime of every
process sharing it. Pass `WithMigrateBucketTTL()` to update such a bucket deliberately; through `factory/coord/leadelect` the switch is
`leaderElector.migrateBucketTTL: true`.

## Shutdown

`Stop` is bounded. It resigns the lease on the way out, and that call is capped by the deadline of the context passed to `Stop`, or by an internal
two-second budget when the context carries none. Without a bound a resignation against an unreachable broker blocks for the NATS driver's own timeout,
regardless of the shutdown budget the caller asked for.

Canceling the context passed to `Start` ends the election the same way. In both cases the node gives up leadership locally before it releases the
key: `IsLeader` reports false and `Fence` reports 0 by the time another node can acquire it, and a node that was leader notifies `LostCh` exactly
once, even when a canceled context is followed at once by `Stop` or a restart. `Stop` returns after that notification and `StopCh` have been
delivered, or after a one-second wait on a consumer that is not reading. `IsRunning` then reports false.

`Start` on a running provider fails. `Start` after the election was ended by its context joins that run — its loss and stop notifications are
delivered by the time `Start` returns — and begins a new one, so no `Stop` is needed in between. Call `Stop` when the provider is no longer needed:
it joins the goroutines of a run ended by its context too.

## Notifications

`BecameCh`, `LostCh` and `StopCh` are delivered by one goroutine that blocks on a consumer that is not reading. Transitions that pile up behind such
a consumer coalesce to the latest state: the consumer always ends on the current one, and a consumer that keeps up sees every edge. Intermediate
edges may be skipped, but a change of term never is. When the node lost leadership and took it again before the consumer was told — or found its key
gone and created it again — another node may have led in between, so a consumer that was told about the earlier term receives `LostCh` and then
`BecameCh` for the new one. `Fence()` read after that `BecameCh` is the new term's token; it also advances with every renewal, so read it when it is
used instead of caching it per notification.

Every term a consumer was told about ends with exactly one `LostCh`, however it ended: a lost lease, a canceled `Start` context or `Stop`. A term the
consumer was not told about yet is never announced after the key has been released.

`LeaderId` answers this node only while `IsLeader` holds; past the lease's freshness bound it reads the key, like a follower.
