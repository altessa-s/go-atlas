# factory

```go
import "github.com/altessa-s/go-atlas/data/mongo/factory"
```

Package `factory` provides a fluent builder for creating a MongoDB cursor storage from configuration.
`CursorStorageBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
storage, err := factory.New(cfg.CursorStorage).
    UseLogger(logger).
    UseRedisClient(redisClient).
    WithTTL(24 * time.Hour).
    Build(ctx)
```

## Supported Storage Types

| Type | Backend | Requires |
|------|---------|----------|
| `memory` | In-process map | `UseScheduler` (optional) |
| `redis` | Redis | `UseRedisClient` |
| `nats` | NATS JetStream KV | `UseJetstream` |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `CursorStorageBuilder` for the given cache storage config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseRedisClient` | Sets the Redis client for Redis storage backends |
| `UseJetstream` | Sets the NATS JetStream context for NATS storage backends |
| `UseScheduler` | Sets the scheduler for background cleanup task registration |

### Configuration

| Method | Description |
|--------|-------------|
| `WithTTL(ttl)` | Sets the TTL applied to all cursor storage entries |

### Terminal

| Method | Description |
|--------|-------------|
| `Build(ctx)` | Assembles and returns the cursor storage |

## NATS bucket

The NATS backend creates its KeyValue bucket (`storage.nats.bucket`, default `cursor`) as file storage with the `WithTTL` key TTL. Without
`WithTTL` the TTL is zero and cursors never expire. An existing bucket is checked like in every other NATS KV backend:

- a different key TTL fails with `ErrBucketTTLMismatch` and leaves the bucket untouched, unless `storage.nats.migrateBucketTTL: true`. Before
  this change the factory silently rewrote the TTL of a shared bucket.
- a different storage type is used as is with a warning, unless `storage.nats.strictBucketStorage: true` makes it fail with
  `ErrBucketStorageMismatch`.
