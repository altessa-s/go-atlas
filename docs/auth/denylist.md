# Token Revocation Denylist

A shared seam for rejecting tokens revoked before their natural expiry. A verifier consults a `Checker` — an in-memory set for a single
process, a Redis store across a fleet, optionally fronted by a probabilistic negative cache — and refuses any identifier it names. Transport-
and JWT-free, keyed by a plain string, so `auth/jwt`, `auth/oidc`, and `selfjwt` can all adopt it.

---

## Table of Contents

- [Overview](#overview)
- [Model](#model)
- [Bounding](#bounding)
- [Quick Start](#quick-start)
  - [In-process denylist](#in-process-denylist)
  - [Distributed store (Redis)](#distributed-store-redis)
  - [Negative cache in front of a distributed store](#negative-cache-in-front-of-a-distributed-store)
- [Correctness and Staleness](#correctness-and-staleness)
- [Observability](#observability)
- [API Reference](#api-reference)
- [Design Notes](#design-notes)
- [See Also](#see-also)

## Overview

`auth/denylist` is the core: a concurrency-safe set of revoked identifiers — a JWT ID (`jti`) for a single token, or a subject for a blanket
ban. A verifier accepts the read-only `Checker` seam (`IsRevoked(key) bool`) so the backing store can start in-memory and later become
distributed without touching the verifier.

```go
import "github.com/altessa-s/go-atlas/auth/denylist"
```

Companion packages extend the seam across a network:

| Package                          | Role                                                                                             |
|----------------------------------|--------------------------------------------------------------------------------------------------|
| `auth/denylist`                  | In-memory exact set + the `Checker` seam. The whole story for a single process.                  |
| `auth/denylist/storages/redis`   | Redis-backed exact store for cross-instance revocation; native per-key TTL, no sweeper.          |
| `auth/denylist/negcache`         | Probabilistic negative cache fronting an authoritative store — never-revoked tokens answered locally. |
| `auth/denylist/mirror`           | Synchronous local snapshot of a distributed store; satisfies `jwt.RevocationChecker` so jwt/selfjwt verifiers enforce distributed revocation with no hot-path network call. |

It complements, rather than replaces, mechanisms a token package already has: `oidc` carries introspection-based revocation, and `selfjwt`
rotates keys. `denylist` is the small explicit seam any of them can share.

## Model

| Concept    | Type / signature                     | Role                                                                              |
|------------|--------------------------------------|-----------------------------------------------------------------------------------|
| Denylist   | `*denylist.Denylist`                 | The in-memory revoked-identifier set, with permanent and expiry-bounded entries.  |
| Checker    | `denylist.Checker` — `IsRevoked(key) bool` | Read seam a verifier accepts; `*Denylist` and any adapter satisfy it.       |
| Authoritative | `negcache.Authoritative` — `IsRevoked(ctx, key) (bool, error)` | The exact store a negative cache defers to.            |

The core distinguishes two entry kinds: **permanent** (`Revoke`, cleared only by `Restore`) and **expiry-bounded** (`RevokeUntil`, forgotten
after its expiry). The common case revokes a token until its own `exp` **plus the verifier's clock-skew leeway** (`auth/jwt.DefaultLeeway`,
30 s; `selfjwt` forwards the same leeway), so the entry outlives the whole window in which a verifier still accepts the token. Revoking only
until `exp` would re-admit a revoked token between `exp` and `exp + leeway`. A token with no bounded expiry needs a permanent `Revoke`.

## Bounding

Expired entries are swept on every write and ignored on reads, so a denylist that only revokes until each token's own expiry stays bounded
without a background goroutine. Unlike a cache it **never** evicts a live (unexpired) entry — that would silently un-revoke a token — so
growth is bounded by TTL expiry and the caller's use of permanent revocations, never by capacity.

## Quick Start

### In-process denylist

```go
import "github.com/altessa-s/go-atlas/auth/denylist"

dl := denylist.New()
// Keep the entry past exp by the verifiers' leeway (and any extra clock difference between instances).
dl.RevokeUntil(claims.ID(), claims.Expiry().Add(jwt.DefaultLeeway))

if dl.IsRevoked(claims.ID()) {
    return ErrRevoked
}
```

Pass `dl` (a `Checker`) to any verifier that accepts the seam. `WithClock(now)` overrides the time source in tests; production uses
`time.Now`.

### Distributed store (Redis)

Each revoked key is a single Redis string (`keyPrefix+key` → `"1"`); Redis expires bounded revocations natively via per-key TTL, so no
sweeper is needed. A `Store` satisfies `negcache.Authoritative` structurally and implements `probfilter.DataLoader`, so it can be both the
exact tier and the source that rebuilds a cache's filter.

```go
import redisstore "github.com/altessa-s/go-atlas/auth/denylist/storages/redis"

store := redisstore.New(client) // client is a redis.UniversalClient
_ = store.RevokeUntil(ctx, "jti-123", time.Until(tokenExp)+jwt.DefaultLeeway) // outlive the verifiers' leeway

revoked, err := store.IsRevoked(ctx, "jti-123")
```

`WithKeyPrefix(string)` overrides the default `denylist:revoked:` namespace.

### Negative cache in front of a distributed store

`negcache` places a Bloom/Cuckoo filter ahead of the authoritative store: a definite filter miss is answered locally (the fast path);
everything else is confirmed against the exact store. The filter has no false negatives, so a revoked key is never fast-pathed as absent;
false positives only cost an extra authoritative lookup and never admit a revoked token.

```go
import "github.com/altessa-s/go-atlas/auth/denylist/negcache"

filter, err := probfilterfactory.NewFilter("denylist", cfg, &defaults).Build()
if err != nil {
    return err
}
cache := negcache.New(filter, store) // store implements Authoritative

// Populate the filter; until a rebuild succeeds the cache defers every lookup to the store.
if err := cache.Rebuild(ctx, store); err != nil {
    return err
}

revoked, err := cache.IsRevoked(ctx, jti) // hot path: skips the network on a definite miss
```

Until the filter is populated by a successful rebuild — `cache.Rebuild(ctx, store)`, a rebuild the filter reports through `LastRebuild`, or, for a
filter shared through Redis, a rebuild committed by any node (`probfilter.RebuildCommitReporter`, checked at most once per
`WithSharedRebuildCheckInterval` while unpopulated) — the cache does not trust the empty filter and answers every `IsRevoked` from the authoritative
store. Then feed it with `cache.Add(ctx, key)` on local revocations and scheduled rebuilds. `negcache.FromChecker(checker)` adapts a synchronous
`denylist.Checker` to `Authoritative` when the exact tier is itself a `Checker`. `cache.Close(ctx)` closes the filter the cache owns.

The factory (`negcache/factory.NewBuilder`) schedules the rebuilds itself: the authoritative store, when it is a `probfilter.DataLoader` (`redis.Store`
is), or a loader set with `UseDataLoader`, rebuilds the Bloom filter per `rebuildOnStart` (inside `Build`) and `rebuildCron` (a process-local cron for
an in-memory filter, a task registered with the `UseScheduler` scheduler for a Redis filter). If another node is rebuilding the shared Redis filter
during `Build`, `Build` still succeeds and the cache defers to the authoritative store until that rebuild is committed.
`authconfig.Denylist.RebuildInterval` (`rebuildInterval`) is for callers that run `cache.Rebuild` themselves; with a Bloom filter it requires
`filter.bloom.rebuildCron: ""`, so one filter never gets two schedules. `redis.Store.StreamValues` is complete only on a single Redis server; with a
Cluster or Ring client it fails with `ErrUnsupportedClient` — inject an exact loader with `UseDataLoader` there.

## Correctness and Staleness

The negative cache is correct only while the filter contains **every** key the authoritative store considers revoked. Maintain that
invariant two ways: `Add` on each local revocation, and a scheduled `Rebuild` from the authoritative source. Between rebuilds a key revoked
on another node is fast-pathed as not-revoked until the next rebuild — the same propagation window any locally cached revocation set has.
Size the rebuild cadence to your revocation-propagation SLA.

`Rebuild` is atomic: the replacement filter is built off to the side while lookups keep using the previous contents, and it takes over in one step. A
failed or canceled rebuild leaves the previous contents (and the populated state) in place, and every `cache.Add` made through the same filter while a
rebuild runs is replayed onto the replacement, so no local revocation is lost. With a filter shared through Redis, `Add`s made by **other** processes
during the rebuild window are not replayed; they reappear at the next rebuild, like any revocation made on another node. Before the first successful
rebuild the cache defers every lookup to the authoritative store.

## Observability

`negcache.WithMetrics(NewMetrics(collector, ""))` records one counter, `lookups_total`, under subsystem `auth_denylist_negcache`:

| Label    | Values                                                                                          |
|----------|-------------------------------------------------------------------------------------------------|
| `result` | `fast_negative` (answered locally), `authoritative_hit`, `authoritative_miss`, `authoritative_error` |
| `filter` | `ok`, `error` (the filter errored and the lookup fell back), `unpopulated` (no successful rebuild yet; filter not consulted) |

The hit rate is `fast_negative / total` — the share of lookups that skipped the round trip; a rising `filter=error` share flags a degraded
filter. A nil collector or `*Metrics` makes every recording a zero-cost no-op. The core `denylist` and the Redis store expose no metrics of
their own.

`mirror.WithMetrics(mirror.NewMetrics(collector, ""))` records, under subsystem `auth_denylist_mirror`, `refreshes_total{result}`
(`ok`/`error`) and the `snapshot_size` gauge — watch a climbing `result="error"` rate for a source going stale.

## API Reference

| Symbol                                       | Description                                                                        |
|----------------------------------------------|------------------------------------------------------------------------------------|
| `denylist.New(opt…)`                         | Empty in-memory denylist. `WithClock(now)` overrides the time source.              |
| `Denylist.Revoke(key)`                       | Deny `key` permanently, until a matching `Restore`.                                |
| `Denylist.RevokeUntil(key, expiry)`          | Deny `key` until `expiry`; an expiry at or before now is a no-op.                  |
| `Denylist.Restore(key)`                      | Re-allow `key`.                                                                    |
| `Denylist.IsRevoked(key) bool`               | Whether `key` is currently revoked (expired entries report false).                |
| `Denylist.Sweep() int` / `Len() int`         | Drop expired entries / tracked-entry count, for observability and tests.           |
| `denylist.Checker`                           | Read seam `IsRevoked(key) bool` a verifier accepts.                                |
| `redisstore.New(client, opt…)`               | Redis-backed exact store; `WithKeyPrefix` sets the namespace.                      |
| `Store.IsRevoked` / `Revoke` / `RevokeUntil` / `Restore` | Context-taking distributed equivalents of the core methods.            |
| `Store.StreamValues(ctx)` / `Count(ctx)`     | SCAN revoked keys (`probfilter.DataLoader`) / count (returns `-1`, unknown).       |
| `negcache.New(filter, authoritative)`        | Negative cache over a filter and an exact store.                                   |
| `Cache.IsRevoked(ctx, key)` / `Add(ctx, key)` / `Rebuild(ctx, loader)` | Lookup / record local revocation / repopulate the filter.        |
| `negcache.FromChecker(checker)`              | Adapt a synchronous `denylist.Checker` to `Authoritative`.                         |
| `negcache.WithMetrics(m)` / `NewMetrics(collector, subsystem)` | Enable lookup telemetry.                                          |

## Design Notes

- **One seam, swappable backing.** The verifier depends only on `Checker` / `Authoritative`, so a deployment moves from in-memory to
  Redis-plus-cache without changing the verifier or the token package.
- **Never un-revoke by eviction.** The core never drops a live entry to bound memory — that would silently re-admit a revoked token. Growth
  is bounded by TTL, matching each token's own expiry plus the verifiers' leeway.
- **Fail toward exactness.** A filter error in `negcache` defers to the authoritative store rather than fast-pathing, so a degraded filter
  costs latency, never correctness.

## See Also

- [`auth/denylist` README](../../auth/denylist/README.md) · [`negcache` README](../../auth/denylist/negcache/README.md) ·
  [`storages/redis` README](../../auth/denylist/storages/redis/README.md) — package quick references.
- [`data/probfilter`](../../data/probfilter) — the Bloom/Cuckoo filter behind `negcache`.
- [`auth/denylist/mirror`](../../auth/denylist/mirror) — synchronous local snapshot of a distributed store for the `jwt.RevocationChecker` seam.
- [oidc.md](oidc.md) · [selfjwt.md](selfjwt.md) — token packages that consult a `Checker`.
- [architecture.md](../architecture.md) — package map and layering.
