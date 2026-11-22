# nats

```go
import "github.com/altessa-s/go-atlas/reliability/requestidstore/storages/nats"
```

A JetStream KV store for `requestid.Guard`. Use one dedicated file-backed bucket per key namespace.

## Atomic operations

`Reserve` reads the current entry through an ordered consumer so a follower cannot return a stale value. It creates an absent key or replaces an expired
entry by revision. `Complete` and `Release` verify the caller token, pending state, and expiry before updating or purging that revision. A CAS conflict
restarts the read. Binary outcomes must fit the NATS server message limit after JSON and base64 encoding.

## Cleanup

Bucket TTL applies to every key. The store requires TTL zero and compares each entry’s `ExpiresAt` with the caller’s `now`. A background pass reclaims
expired values and release markers through a subject purge bounded by the observed revision. For values, the cutoff is the latest caller time
observed by the instance minus the cleanup grace. Set the grace above the maximum skew between callers sharing the bucket. Idle buckets may retain
expired data until another operation supplies a later time. The connection needs permission to watch the bucket and purge its stream by subject.
Call `Close(ctx)` to stop the loop before closing the connection.

## Options

| Option              | Default     | Purpose                                        |
|---------------------|-------------|------------------------------------------------|
| `WithCleanupGrace`  | `1h`        | Extra retention for clock skew (minimum 1 ms). |
| `WithBucket`        | `requestid` | Dedicated KV bucket.                           |
| `WithReplicas`      | `1`         | Replica count for a newly created bucket.      |
| `WithPurgeInterval` | `1m`        | Delay between passes. Zero disables cleanup.   |
| `WithLogger`        | Discard     | Logs bucket setup and background purge errors. |

See [requestidstore](../../README.md) for the Guard API and the other backends.
