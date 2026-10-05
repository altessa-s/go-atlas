# negcache

```go
import "github.com/altessa-s/go-atlas/auth/denylist/negcache"
```

Probabilistic **negative cache** in front of an authoritative (typically distributed) revocation store. A definite filter miss is answered
locally; everything else is confirmed against the exact store — so the hot path (a token that was never revoked) avoids a network round
trip, without the false-positive rejections a bare filter would cause.

It complements [`auth/denylist`](../README.md): the in-memory `Denylist` is the exact store for a single process, while `negcache` is the
read-path accelerator when that exact store lives across the network.

## How it works

| Filter result | Action |
|---------------|--------|
| not yet populated (no successful rebuild) | consult the `Authoritative` store — an empty filter proves nothing |
| definitely absent | return `false` locally (fast path) |
| possibly present | consult the `Authoritative` store for the exact answer |
| filter error | consult the `Authoritative` store (fail toward exactness) |

A Bloom/Cuckoo filter has **no false negatives**, so a revoked key is never fast-pathed as absent — as long as the filter stays a superset
of the revoked set. Filter **false positives** only cost an extra authoritative lookup; they never admit a revoked token nor reject a valid
one, because the authoritative store makes the final call.

## Types

| Type | Purpose |
|------|---------|
| `Cache` | The negative cache: wraps a `probfilter.Filter` and an `Authoritative`. |
| `Authoritative` | Consumer-side interface for the exact store: `IsRevoked(ctx, key) (bool, error)`. |

## API

| Method | Purpose |
|--------|---------|
| `New(filter, authoritative)` | Construct a cache over a filter and exact store. |
| `Cache.IsRevoked(ctx, key)` | Lookup: fast-path a definite miss, else defer to the authoritative store. |
| `Cache.Add(ctx, key)` | Record a locally revoked key in the filter (keeps the superset invariant between rebuilds). |
| `Cache.Rebuild(ctx, loader)` | Repopulate the filter from the authoritative full key stream (atomic); the first success marks the cache populated. Requires a `probfilter.RebuildableFilter` (Bloom or Cuckoo). |
| `Cache.Close(ctx)` | Close the owned filter (Bloom/Cuckoo facade or `io.Closer`); stops rebuilds a factory scheduled for it. |
| `FromChecker(checker)` | Adapt a synchronous [`denylist.Checker`](../README.md) to `Authoritative`, so any Checker (in-process or distributed) can be the exact tier. |
| `WithMetrics(m)` | Option enabling lookup telemetry via a `*Metrics` (see [Metrics](#metrics)). |
| `WithSharedRebuildCheckInterval(d)` | How often, at most, an unpopulated cache asks a shared filter whether any process committed a rebuild (default `DefaultSharedRebuildCheckInterval`, 1s; `0` checks on every unpopulated lookup). |
| `NewMetrics(collector, subsystem)` | Build a `*Metrics` over an [`observability/metrics`](../../../observability/metrics) collector; a nil collector or `*Metrics` is a no-op. |

## Invariant & staleness

The cache is correct only while the filter contains **every** key the authoritative store considers revoked. Until the first successful `Rebuild`
(through the cache, one the filter reports via `LastRebuild`, or — for a filter shared through Redis — one committed by any process, reported via
`probfilter.RebuildCommitReporter`) the cache answers every lookup from the authoritative store. The shared check costs a Redis round trip, runs at most
once per `WithSharedRebuildCheckInterval` while unpopulated, and an error counts as unpopulated. Rebuilds are atomic:
lookups use the previous contents while one runs, a failed rebuild keeps them, and `Add` calls made during a rebuild are kept. Maintain the invariant
with `Add` on local revocations and a scheduled `Rebuild` from the authoritative source. Between rebuilds a key revoked on another node is fast-pathed
as not-revoked until the next rebuild — the same propagation window any locally cached revocation set has. Size the rebuild cadence to your
revocation-propagation SLA.

## Metrics

Pass `WithMetrics(NewMetrics(collector, ""))` to record one counter, `lookups_total`, labelled by `result` and `filter`:

| Label | Values |
|-------|--------|
| `result` | `fast_negative` (answered locally), `authoritative_hit`, `authoritative_miss`, `authoritative_error` |
| `filter` | `ok`, `error` (the negative filter errored and the lookup fell back), `unpopulated` (no successful rebuild yet; filter not consulted) |

The hit rate is `fast_negative / total` — the share of lookups that skipped the authoritative round trip. A rising `filter=error` share flags a
degraded filter. Metrics are optional: a nil collector or `*Metrics` makes every recording a zero-cost no-op.

## Usage

The [factory](factory/) builds the filter and lets the authoritative store drive its rebuilds (`rebuildOnStart`, `rebuildCron`). Assembled by hand:

```go
filter, _ := probfilterfactory.NewFilter("denylist", cfg, &defaults).Build()
cache := negcache.New(filter, redisDenylist) // redisDenylist implements Authoritative
defer cache.Close(ctx)

revoked, err := cache.IsRevoked(ctx, jti) // hot path

_ = cache.Rebuild(ctx, redisDenylist) // periodic repopulation; the store is a probfilter.DataLoader
```

See also: [`data/probfilter`](../../../data/probfilter), [`auth/denylist`](../README.md).
