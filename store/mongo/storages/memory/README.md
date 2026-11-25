# memory

```go
import "github.com/altessa-s/go-atlas/store/mongo/storages/memory"
```

Package `memory` provides an in-memory cursor storage for MongoDB pagination state. Suitable for single-instance deployments and testing where cursor
persistence across restarts is not required.

`New(ttl, opts...)` falls back to `DefaultTTL` (1h) when `ttl` is not positive. Expired cursors are removed by `RunCleanup`, called manually or
scheduled via `WithScheduler` and `WithCleanupSchedule`, and by `Store` itself, which drops the expired cursors at the head of its insertion order.
The TTL is the same for every cursor, so the insertion order is the expiry order.

The store snapshots the input metadata and returns a fresh copy on every successful non-nil load. A stored nil loads as nil.

## Entry caps

| Option                     | Default                                               | Bounds                                                      |
|----------------------------|-------------------------------------------------------|-------------------------------------------------------------|
| `WithMaxEntries`           | `DefaultMaxEntries` (100 000)                         | All stored cursors; values below 1 are ignored              |
| `WithMaxEntriesPerSubject` | A tenth of `WithMaxEntries`, from 1 to 10 000         | Cursors bound to one subject; 0 disables, negatives ignored |

Every page that has a successor stores a cursor, so `WithMaxEntries` is what keeps a client paging in a loop from growing the map without bound. At
the cap `Store` evicts the oldest cursors, which then resolve to `mongo.ErrCursorNotFound`. The caps count the cursors that are stored: a deleted,
expired or replaced cursor frees its place at once and leaves nothing behind in the eviction order.

`WithMaxEntriesPerSubject` keeps one principal from spending the whole cap. The storage reads the subject from the metadata it is given
(`mongo.WithListCursorSubject`); a subject over its cap loses its own oldest cursors and the other subjects keep theirs. When the option is not given,
the cap follows the global one: a tenth of `WithMaxEntries`, at least 1 and at most `DefaultMaxEntriesPerSubject` (10 000), so `WithMaxEntries(1000)`
alone caps each subject at 100. An explicit value is used as given, 0 included; one above `WithMaxEntries` is the same as `WithMaxEntries` and
separates no one, so set it well below.

**Limitation:** cursors without a subject are held to `WithMaxEntries` only, so in a deployment that does not pass a subject one client paging in a
loop can still evict the cursors of every other caller. The subject is also whatever the service passes: bound to a tenant, the cap does not separate
the users of that tenant. For multi-tenant deployments prefer the [redis](../redis) or [nats](../nats) storage, which bound cursors by their TTL
rather than by an entry cap.
