# factory

```go
import "github.com/altessa-s/go-atlas/data/idempotency/factory"
```

Package `factory` provides a fluent builder for creating an idempotency keeper from configuration. `KeeperBuilder` uses deferred error accumulation —
errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
keeper, err := factory.New(cfg.Idempotency).
    UseLogger(logger).
    UseRedisClient(redisClient).
    Build()
```

## Supported Storage Types

| Type | Backend | Requires |
|------|---------|----------|
| `memory` | In-process map with TTL | `UseScheduler` (optional) |
| `redis` | Redis | `UseRedisClient` |
| `nats` | NATS JetStream KV | `UseJetstream` |
| `mongo` | MongoDB collection, joins the caller's transaction | `UseMongoDatabase` |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `KeeperBuilder` for the given idempotency config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseDefaultLogger` | Sets the logger to `slog.Default()` |
| `UseRedisClient` | Sets the Redis client for Redis storage backends |
| `UseJetstream` | Sets the NATS JetStream context for NATS storage backends |
| `UseMongoDatabase` | Sets the MongoDB database for the mongo storage backend |
| `UseScheduler` | Sets the scheduler for background cleanup task registration |
| `UseCollector` | Sets the metrics collector passed to the keeper |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the idempotency keeper |
