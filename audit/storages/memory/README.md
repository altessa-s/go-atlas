# memory

```go
import "github.com/altessa-s/go-atlas/audit/storages/memory"
```

Package `memory` provides a bounded in-memory ring-buffer implementation of `audit.Storage`. All data lives in process memory and is lost on exit; when
capacity is reached, storing new events evicts the oldest stored events.

## Key types

| Type / Function    | Description                                                               |
|--------------------|---------------------------------------------------------------------------|
| `Storage`          | Thread-safe bounded ring buffer implementing `audit.Storage`              |
| `New(opts...)`     | Creates a bounded `Storage` (`DefaultMaxEvents` = 10,000)                 |
| `DefaultMaxEvents` | Default ring-buffer capacity (10,000 events)                              |

## Options

| Option          | Default | Description                                                               |
|-----------------|---------|---------------------------------------------------------------------------|
| `WithMaxEvents` | `10000` | Maximum number of events retained before evicting the oldest (`< 1` no-op) |

## Behavior

- `Store` and `StoreBatch` reject `nil` events with `coreerrs.ErrInvalidArgument` before mutating or evicting stored events; an empty batch succeeds.
- Evicted slots and `Reset()` clear their `*audit.Event` references so evicted events can be garbage-collected immediately.
- `Len()` reports the current number of retained events and `Cap()` reports the configured capacity.

See the [parent storages overview](../README.md) and [`audit`](../../README.md).
