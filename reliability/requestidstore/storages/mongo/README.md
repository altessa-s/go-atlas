# mongo

```go
import "github.com/altessa-s/go-atlas/reliability/requestidstore/storages/mongo"
```

Stores [AIP-155 request IDs](../..) in a dedicated MongoDB collection. Pass the returned store to `requestid.New`.

## Options

| Option               | Default       | Purpose                                            |
|----------------------|---------------|----------------------------------------------------|
| `WithCleanupGrace`   | `1h`          | Extra retention for clock skew (minimum 1 ms).     |
| `WithCollectionName` | `request_ids` | Select the collection holding one document per key |

## Atomic operations

`Reserve` uses one find-and-modify upsert. Its update pipeline inserts into an empty slot, replaces a document at or after its logical deadline, or
returns the live entry. The pre-image identifies whether the caller acquired the reservation even if another process supplied an identical entry.

`Complete` and `Release` condition their writes on the pending state, exact token bytes, and a deadline after the caller's `now`. Completion stores the
new outcome with its own deadline. MongoDB stores dates with millisecond precision.

## Deployment

Use MongoDB 4.2 or later, acknowledged writes, and a dedicated, uncapped collection with the default simple collation. Startup ensures a TTL index on
`purge_at`. Its timestamp follows the server clock plus the remaining caller-supplied duration and the configured cleanup grace.
Set the grace above the maximum amount by which elapsed server time can exceed elapsed caller time, including a change of caller replica.
Logical takeover and replay work while expired documents await the TTL monitor.
MongoDB's 16 MiB document limit applies to each encoded entry.
