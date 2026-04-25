# uniq

```go
import "github.com/altessa-s/go-atlas/data/uniq"
```

Package `uniq` provides unique value management with multiple storage backends (Redis, NATS, no-op). Supports adding, checking existence,
retrieving, and removing unique keys with optional associated values. Default TTL is 24 hours.

## Options

| Option | Default | Description |
|---|---|---|
| `WithSerializer` | JSON | Serialization format |
| `WithCollector` | noop | Prometheus metrics collector |
| `WithLogger` | discard | Structured logger (used for health probe debug) |
| `WithHealthCoordinator` | unset | Auto-register with `health.Coordinator` on construction |
| `WithHealthServiceName` | `"uniq"` | Override service name when several Uniq instances share a coordinator |

## Add vs TryAdd

`Add` / `AddWithValue` always overwrite — fine for "mark this done" writes after the work succeeded. `TryAdd` / `TryAddWithValue` are the
race-free CAS variants: they return `(true, nil)` only when the key didn't exist. Use `TryAdd` for first-writer-wins flows (one-time tokens,
idempotent webhooks, duplicate event suppression). Backed by Redis `SET NX` and NATS JetStream `KV.Create`.

## Metrics

Subsystem `uniq`. Every public method is instrumented with the `op` label:
`add`, `add_with_value`, `try_add`, `try_add_with_value`, `exist`, `get_value`, `remove`, `clear`.

| Metric | Type | Description |
|---|---|---|
| `uniq_operations_total` | counter | Operations performed |
| `uniq_operation_duration_seconds` | histogram | Operation duration |
| `uniq_operation_errors_total` | counter | Operation failures |

## Health

`*Uniq` implements `health.Checker`. Pass `WithHealthCoordinator(c)` and `Uniq` registers itself under the configured service name (default
`"uniq"`). Health probes delegate to `providers.Prober` when the provider implements it; in-tree providers (`nats`, `redis`, `noop`) all do.
NATS probes via `kv.Status(ctx)`, Redis via `client.Ping(ctx)`, noop is always healthy.

## Subpackages

| Package | Description |
|---|---|
| [factory](./factory) | Configuration-based creation |
| [providers/redis](./providers/redis) | Redis-backed provider |
| [providers/nats](./providers/nats) | NATS KV provider |
| [providers/noop](./providers/noop) | No-op provider for testing |
