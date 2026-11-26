# mongo

```go
import "github.com/altessa-s/go-atlas/audit/storages/mongo"
```

Package `mongo` implements `audit.Storage` using MongoDB as the backing store. Supports unordered idempotent batch writes, indexed queries, and TTL-based
cleanup for durable audit trail persistence.

An empty batch succeeds. A nil event in `Store` or `StoreBatch` fails with `coreerrs.ErrInvalidArgument` before any event is written. Replay detection is
keyed by `Event.ID` (`_id`) only — callers invoking `Storage` directly without `Auditor` must supply unique non-empty IDs. On code `11000`, `Store` and
`StoreBatch` verify `_id` existence on the primary with `majority` read concern and acknowledge only verified replays. Write-concern failures from any
sub-batch wrap `coreerrs.ErrUnavailable`, unverified unique-constraint violations wrap `coreerrs.ErrConflict`, and non-duplicate write rejections wrap
`coreerrs.ErrFailedPrecondition`, always without raw MongoDB server text. Positive TTLs must be between `MinTTL` (`1s`) and `MaxTTL` (`math.MaxInt32`
seconds).

## Indexes

`New` creates these indexes:

| Keys                                           | Serves                               |
|------------------------------------------------|--------------------------------------|
| `timestamp -1, _id -1` (simple collation)      | keyset paging and time-range queries |
| `timestamp -1, type 1`                         | time-ordered queries by event type   |
| `actor.id 1, timestamp -1`                     | queries by actor                     |
| `resource.type 1, resource.id 1, timestamp -1` | queries by resource                  |
| `context.request_id 1` (sparse)                | queries by request ID                |
| `context.trace_id 1` (sparse)                  | queries by trace ID                  |
| `timestamp 1` (TTL, with `WithTTL`)            | retention                            |

Index creation is bounded by the index timeout.
