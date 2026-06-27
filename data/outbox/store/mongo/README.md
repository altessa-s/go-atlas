# mongo

```go
import "github.com/altessa-s/go-atlas/data/outbox/store/mongo"
```

Package `mongo` implements `outbox.Store` using MongoDB as the backing store. Provides durable event persistence with indexed queries for
efficient batch fetch, status updates, and cleanup.

## Server clock

All time-based predicates (retry-after, lock-expiry, retention, expiry) are evaluated against the **MongoDB server clock** via the `$$NOW` aggregation
variable inside `$expr` filters and pipeline updates — never the application's wall clock. A worker with a skewed clock therefore cannot prematurely
unlock another worker's in-flight event or leak a stuck one: every comparison uses the single authoritative clock shared by all workers. Timestamps are
stored as BSON `Date` (required for `$$NOW` arithmetic), with absent optional timestamps represented as missing fields rather than the Unix epoch.

## Migration

Legacy collections stored timestamps as int64 Unix seconds. Run the in-place, idempotent migration **once per collection** before serving traffic with
this store, then wire it into your migration registry (do not call it from `New`):

| Function                                       | Direction                          |
|------------------------------------------------|------------------------------------|
| `MigrateTimestampsToDate(ctx, collection)`     | int64 Unix seconds → BSON `Date`   |
| `RevertTimestampsToUnix(ctx, collection)`      | BSON `Date` → int64 Unix seconds (rollback) |

Both return the number of documents converted, only touch documents still in the source layout (so re-running is a no-op), and drop optional
timestamps that are zero/absent rather than mapping them to `1970`.
