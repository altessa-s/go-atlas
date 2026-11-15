# natskvlease

```go
import "github.com/altessa-s/go-atlas/internal/nats/natskvlease"
```

Shared NATS KeyValue lease and migration utilities for the data packages. Provides revision-based ownership, renewal, retry and resumable bucket
storage migration.

## Key types

| Type | Purpose |
|------|---------|
| `Lease` | Acquires, renews and releases revision-fenced leases |
| `KVHelper` | Creates buckets and migrates their storage |
| `BucketSpec` | Desired state of a bucket, reconciled by `GetOrCreateBucket` and targeted by `MigrateBucketStorage` |
| `MigrationOptions` | Controls migration and resuming an interrupted operation |

## Options

`NewLease(kv, key, ttl, value, opts...)` takes the lease key, lifetime and initial value as arguments; the rest is optional.

| Option | Default | Description |
|--------|---------|-------------|
| `WithLeaseRenewRatio` | `DefaultRenewRatio` | Fraction of the lifetime at which the lease is renewed; values outside (0, 1] fall back to the default |
| `WithLeaseOwnerCheck` | none | Reports whether a stored value belongs to this holder |
| `WithLeaseCallbacks` | none | `LeaseCallbacks` fired on acquire, renew, loss and release |
| `WithLeaseLogger` | `slog.Default()` | Logger for lease operations; nil keeps the default |
| `WithLeaseCollector` | none | Meter for the lease metrics; nil disables them |

`Retry(ctx, fn, opts...)` retries transient network errors with exponential backoff.

| Option | Default | Description |
|--------|---------|-------------|
| `WithRetryMaxAttempts` | `DefaultMaxRetries` | Total attempts, the first call included; zero behaves like one |
| `WithRetryMaxElapsedTime` | `DefaultMaxElapsedTime` | Time budget across all attempts |
| `WithRetryJitter` | `DefaultJitter` | Fraction of each delay added as randomization; zero is deterministic |

## Migration TTLs

Source and marker headers share validation of whole-second TTLs. Negative numeric values and values exceeding `time.Duration` are rejected;
`never` is the explicit no-expiration marker. Duration strings are truncated to whole seconds without floating-point conversion.
