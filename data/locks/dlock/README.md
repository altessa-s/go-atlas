# dlock

```go
import "github.com/altessa-s/go-atlas/data/locks/dlock"
```

Package `dlock` provides distributed locks for coordinating access across multiple service instances. Supports pluggable providers (NATS
JetStream, no-op) with automatic resource management and context-aware operations.

## Options

| Option                   | Default | Description                        |
|--------------------------|---------|------------------------------------|
| `WithLockAcquireTimeout` | 30s     | Timeout for lock acquisition       |
| `WithLogger`             | discard | Structured logger                  |

## Subpackages

| Package                              | Description                          |
|--------------------------------------|--------------------------------------|
| [factory](./factory)                 | Configuration-based creation         |
| [providers/nats](./providers/nats)   | NATS JetStream provider              |
| [providers/noop](./providers/noop)   | No-op provider for testing           |
| [errs](./errs)                       | Error definitions                    |
