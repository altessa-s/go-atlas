# natskvlease

```go
import "github.com/altessa-s/go-atlas/internal/nats/natskvlease"
```

Shared NATS KeyValue lease utilities for the data packages. Provides revision-based ownership, renewal, retry and bucket creation.

## Key types

| Type | Purpose |
|------|---------|
| `Lease` | Acquires, renews and releases revision-fenced leases |
| `KVHelper` | Creates buckets and reconciles existing ones against a `BucketSpec` |
| `BucketSpec` | Desired state of a bucket, reconciled by `GetOrCreateBucket` |

## Options

`NewLease(kv, key, ttl, value, opts...)` takes the lease key, lifetime and initial value as arguments; the rest is optional.

| Option | Default | Description |
|--------|---------|-------------|
| `WithLeaseRenewRatio` | `DefaultRenewRatio` | Fraction of the lifetime at which the lease is renewed; values outside (0, 1] fall back to the default |
| `WithLeaseOwnerCheck` | none | Reports whether a stored value belongs to this holder |
| `WithLeaseCallbacks` | none | `LeaseCallbacks` fired on acquire, renew, loss and release |
| `WithLeaseLogger` | `slog.Default()` | Logger for lease operations; nil keeps the default |
| `WithLeaseObserver` | none | Receives operation results and held-state transitions |

`Retry(ctx, fn, opts...)` retries transient network errors with exponential backoff.

Lease acquisition rejects acknowledgments at or beyond the TTL minus the renewal safety margin. It cleans up uncertain writes using the owner and KV
revision. If renewal ends with uncertain ownership, either `Release` or cancellation of the caller's context can still remove the key conditionally.
`OnLost` runs after renewal has stopped and can call `Release`.

| Option | Default | Description |
|--------|---------|-------------|
| `WithRetryMaxAttempts` | `DefaultMaxRetries` | Total attempts, the first call included; zero behaves like one |
| `WithRetryMaxElapsedTime` | `DefaultMaxElapsedTime` | Time budget across all attempts |
| `WithRetryJitter` | `DefaultJitter` | Fraction of each delay added as randomization; zero is deterministic |
