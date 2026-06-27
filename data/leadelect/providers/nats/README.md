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
| `WithRenewRatio` | `0.75`                   | Fraction of the lease TTL at which the lease is renewed.                                  |
| `WithStorage`    | `jetstream.MemoryStorage`| JetStream storage backend for the bucket. Use `jetstream.FileStorage` to survive restarts.|
| `WithCollector`  | no-op                    | Metrics collector.                                                                       |
| `WithLogger`     | discard                  | Structured logger.                                                                       |

Memory storage keeps the lease ephemeral (lost on a JetStream restart, forcing a clean re-election) and avoids disk I/O on the renew hot path. Choose
`WithStorage(jetstream.FileStorage)` when the bucket — and the monotonic fencing revision behind `Fence()` — must survive a full server bounce.
