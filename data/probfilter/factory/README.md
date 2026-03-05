# factory

```go
import "github.com/altessa-s/go-atlas/data/probfilter/factory"
```

Package `factory` provides fluent builders for creating probabilistic filters and their manager from configuration.
Both `FilterBuilder` and `ManagerBuilder` use deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
// Build a single filter
filter, err := factory.NewFilter("my-filter", cfg.Filter, cfg.Defaults).
    UseLogger(logger).
    UseRedisClient(redisClient).
    Build()

// Build a manager with all configured filters
manager, err := factory.NewManager(cfg.ProbabilisticFilter).
    UseLogger(logger).
    UseRedisClient(redisClient).
    Build()
```

## Supported Filter Types

| Type | Description |
|------|-------------|
| `bloom` | Bloom filter — space-efficient membership testing with tunable false positive rate |
| `cuckoo` | Cuckoo filter — supports deletion with configurable capacity |

## Supported Storage Types

| Type | Backend | Requires |
|------|---------|----------|
| `memory` | In-process | — |
| `redis` | Redis | `UseRedisClient` |

---

## FilterBuilder

### Constructor

| Method | Description |
|--------|-------------|
| `NewFilter(name, cfg, defaults)` | Creates a `FilterBuilder` for the given filter name, config, and defaults |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseRedisClient` | Sets the Redis client for Redis-backed filter storages |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the probabilistic filter |

---

## ManagerBuilder

### Constructor

| Method | Description |
|--------|-------------|
| `NewManager(cfg)` | Creates a `ManagerBuilder` for the given probabilistic filter config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the manager builder and all created components |
| `UseRedisClient` | Sets the Redis client for Redis-backed filter storages |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the probabilistic filter manager with all configured filters |
