# nats

```go
import "github.com/altessa-s/go-atlas/data/leadelect/providers/nats"
```

Package `nats` implements leader election using NATS JetStream key-value store with TTL-based lease renewal. Provides distributed leader election
without external dependencies beyond NATS.

## Options

| Option           | Default                  | Description                                                                              |
|------------------|--------------------------|------------------------------------------------------------------------------------------|
| `WithBucket`     | `leadelect`              | KeyValue bucket name for the election key.                                               |
| `WithRenewRatio` | `1/3`                    | Fraction of the lease lifetime at which the lease is renewed.                             |
| `WithStorage`    | `jetstream.MemoryStorage`| JetStream storage backend for the bucket. Use `jetstream.FileStorage` to survive restarts.|
| `WithCollector`  | no-op                    | Metrics collector.                                                                       |
| `WithLogger`     | discard                  | Structured logger.                                                                       |

Memory storage keeps the lease ephemeral (lost on a JetStream restart, forcing a clean re-election) and avoids disk I/O on the renew hot path. Choose
`WithStorage(jetstream.FileStorage)` when the bucket — and the monotonic fencing revision behind `Fence()` — must survive a full server bounce.

## Lease renewal

Renewals tick every `min(electionTTL, bucketKeyTTL) × renewRatio`. The ratio therefore decides how many renewal attempts fall inside one lease
lifetime — `floor(1/ratio)` of them — and that count is the budget for surviving a transient NATS failure without a spurious re-election. At `0.75`
the budget is a single attempt, so one dropped packet costs leadership; at `1/3` three ticks land inside the window, two of them with room to spare.
One third is the usual choice for lease keepalives (etcd sessions and ZooKeeper heartbeats both renew at `TTL/3`).

A ratio outside `(0, 1]` falls back to the default: a non-positive interval would make `time.NewTicker` panic inside the camping goroutine, where the
recover would swallow it and leave an elector that reports itself running while never electing anyone.

## Bucket TTL

The bucket's key TTL is the expiry mechanism behind the lease: it is what releases the election key when a holder dies without resigning. The provider
therefore reconciles the TTL of a bucket it adopts — a pre-existing bucket created without one would otherwise yield leases that never expire, hanging
the election until someone intervenes by hand.

## Shutdown

`Stop` is bounded. It resigns the lease on the way out, and that call is capped by the deadline of the context passed to `Stop`, or by an internal
two-second budget when the context carries none. Without a bound a resignation against an unreachable broker blocks for the NATS driver's own timeout,
regardless of the shutdown budget the caller asked for.
