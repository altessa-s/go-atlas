# clickhouse

```go
import auditclickhouse "github.com/altessa-s/go-atlas/data/audit/storages/clickhouse"
```

ClickHouse-backed [`audit.Storage`](../../storage.go). Audit traffic is append-only, high-volume, and read back analytically, which is the
workload ClickHouse is built for: events land in one wide table, partitioned by month and sorted so that the common query shape — a time
range, then an actor — reads as few granules as possible.

The connection is injected and stays owned by the caller, so the package pulls in no client lifecycle of its own.

## Options

| Option                  | Default              | Description                                                                              |
|-------------------------|----------------------|------------------------------------------------------------------------------------------|
| `WithTableName`         | `audit_events`       | Table holding the events.                                                                 |
| `WithEngine`            | `ReplacingMergeTree` | ENGINE clause used by `SchemaDDL`: a MergeTree-family engine whose parameters are literals. |
| `WithCluster`           | none                 | ON CLUSTER name for the DDL. Pair it with a `Replicated*` engine.                          |
| `WithTTL`               | none                 | Row-level TTL. Second granularity; a sub-second value is raised to one second.             |
| `WithMaxBatchSize`      | `10000`              | Largest number of rows per INSERT. Bigger inputs are split across several batches.         |
| `WithAutoCreateTable`   | off                  | Run `SchemaDDL` from `New`. Off by default — schema changes belong in a migration.         |
| `WithDDLTimeout`        | `30s`                | Bound on the CREATE TABLE that `WithAutoCreateTable` issues from `New`.                    |
| `WithFinal`             | off                  | Add FINAL to reads so they observe the deduplicated view of a `ReplacingMergeTree` table.  |
| `WithTimeRangeMode`     | `warn`               | Reaction to a query that bounds no time range: `enforce`, `warn`, or `disabled`.           |
| `WithLogger`            | discard              | Logger for the time-range warning.                                                         |

## Key types

| Symbol                                | Description                                                                                            |
|---------------------------------------|--------------------------------------------------------------------------------------------------------|
| `Storage`                             | Implements `audit.Storage`. Construct with `New(conn, opts...)`.                                        |
| `Conn`                                | The four driver methods the storage uses. `driver.Conn` satisfies it; tests substitute a double.        |
| `SchemaDDL(table, engine, cluster, ttl)` | CREATE TABLE statement, so a migration can own the schema. Fails on an unsafe name.               |
| `TimeRangeMode`                       | `TimeRangeModeEnforce` / `TimeRangeModeWarn` / `TimeRangeModeDisabled`.                                 |
| `ErrNoConn`                           | Returned by `New` when no connection is supplied.                                                       |
| `ErrInvalidIdentifier`                | Returned by `New` and `SchemaDDL` when the table or cluster name is not a plain identifier.             |
| `ErrInvalidEngine`                    | Returned by `New` and `SchemaDDL` when the engine is not a MergeTree-family engine with literal params. |
| `ErrTimeRangeRequired`                | Returned by `Query` and `Count` under `TimeRangeModeEnforce`.                                           |

## Schema

Fields that queries filter on become typed columns; free-form maps (event metadata, actor metadata, resource attributes, resource changes)
are carried as JSON strings so the table stays fixed-width without a schema per tenant.

| Clause         | Value                                                    |
|----------------|----------------------------------------------------------|
| `PARTITION BY` | `toYYYYMM(timestamp)`                                     |
| `ORDER BY`     | `(toDate(timestamp), actor_id, timestamp, id)`            |
| `TTL`          | `toDateTime(timestamp) + INTERVAL <ttl> SECOND`, if set   |
| `ON CLUSTER`   | added when `WithCluster` is set                           |

Three bloom-filter skip indexes cover the lookups the sorting key cannot serve:

| Index             | Column       | Answers                                  |
|-------------------|--------------|------------------------------------------|
| `idx_trace_id`    | `trace_id`   | "what did this trace do"                  |
| `idx_request_id`  | `request_id` | "what did this request do"                |
| `idx_resource_id` | `resource_id`| "show me the history of this object"      |

Without them those queries read every granule of every partition in range, since none of the three columns is part of the sorting key.

## Migrations

There are no versioned migrations, and that is deliberate. MongoDB in this repo gets versioned migrations (`data/mongo/migration.go`) and
unconditional index creation on every `New`, because `createIndexes` is idempotent and cheap. ClickHouse is not analogous: `CREATE TABLE IF NOT
EXISTS` does **nothing** to a table that already exists, so it silently fails to add a column or an index introduced after the table was created.
Running it on every startup would give a false sense that the schema is current.

What the storage offers instead is three explicit steps: a drift check (`CheckSchema`), an opt-in additive repair of what is missing
(`MigrateSchema`), and an operator-run backfill of new indexes over existing parts (`MaterializeIndexes`).

The drift check compares `system.tables`, `system.columns` and `system.data_skipping_indices` against what this package
expects and reports the difference: missing columns and skip indexes, columns of another type, skip indexes on another expression or of another type or
granularity, and another sorting or partition key. Expressions are compared after removing whitespace and one pair of enclosing parentheses, since
ClickHouse renders them its own way. TTL is not compared: it is configurable and an existing table may legitimately differ. Additive migration repairs
only what is missing; a differently defined column, index or key stays reported until it is fixed by hand.

```go
if err := storage.CheckSchema(ctx); err != nil {
    return err
}
```

| `WithSchemaCheck` | Behavior |
|-------------------|----------------------------------------------------|
| `enforce`         | Returns `ErrSchemaMismatch` naming what differs      |
| `warn` (default)  | Logs what differs and continues                      |
| `disabled`        | Skips the check and its system-table queries         |

`New` never runs the check. Unless `WithAutoCreateTable` is set, constructing a storage does no I/O, matching the driver's own lazy connect.
[`data/audit/factory`](../../factory) calls it when it builds the storage, which is where a startup round trip already happens; wire it
yourself if you construct the storage directly.

### Repairing the drift

`Storage.MigrateSchema(ctx)` adds what is missing, and `data/audit/factory` calls it before the check — but only when the configuration
opts in:

| `WithSchemaMigration` | Behavior |
|-----------------------|--------------------------------------------------------------|
| `off` (default)       | No-op. Changing a table is an act to opt into.                 |
| `additive`            | `ADD COLUMN IF NOT EXISTS` and `ADD INDEX IF NOT EXISTS` only. |

What makes `additive` safe is that both statements are **metadata-only**: they change the table definition and do not touch a single
existing part. Nothing is modified or dropped, so there is nothing to lose and no down direction. `IF NOT EXISTS` means replicas starting
together do not fight over the same statement.

On a `Replicated*` table the statements must reach every replica, so a cluster is required. Without one, `MigrateSchema` returns
`ErrUnsafeMigration` rather than adding a column to one replica and not the others — schema skew is worse than the drift it would be fixing.

### Backfilling an index over existing parts

A newly added index covers only parts written after it was added. Older parts stay unindexed, which costs speed and not correctness — and
for an audit table with a TTL they age out anyway. Backfilling them is `Storage.MaterializeIndexes(ctx)`, and it is deliberately separate
from everything above:

```go
// Explicit, once, by an operator. Nothing calls this on your behalf.
if err := storage.MaterializeIndexes(ctx); err != nil {
    return err
}
```

`MATERIALIZE INDEX` is a mutation: ClickHouse rebuilds the index over existing parts in the background, at a cost that scales with the
column being indexed and competes for disk with the inserts the audit trail depends on. Neither `New`, nor the factory, nor
`MigrateSchema` will ever issue it. The call returns as soon as the statements are accepted; watch `system.mutations` for progress.

The deciding reason it is not automated is not the cost but the shape: there is no `IF NOT MATERIALIZED`. The additive statements are free
to repeat on every startup of every replica; a materialization is not, and would queue one mutation per process.

`CheckSchema` does warn when it looks needed — an index reporting zero `data_compressed_bytes` over a table that holds rows is what an
index added after the fact looks like. That is a heuristic and never an error: ClickHouse exposes no per-part flag for it, and a partially
materialized index already reports a non-zero size.

**Adding indexes to a table that already exists.** `SchemaDDL` only describes a new table — `CREATE TABLE IF NOT EXISTS` will not alter one
that is already there, so a table created before these indexes existed keeps none of them. Adding an index is two statements: the first
registers it for data written from now on, the second backfills the parts already on disk.

```sql
ALTER TABLE audit_events ADD INDEX idx_resource_id resource_id TYPE bloom_filter(0.01) GRANULARITY 4;
ALTER TABLE audit_events MATERIALIZE INDEX idx_resource_id;
```

`MATERIALIZE INDEX` is a mutation: it rewrites every affected part in the background and can take a long time on a large table. Watch
`system.mutations` and expect the disk and CPU cost while it runs. On a cluster, add `ON CLUSTER <name>` to both statements.

Changing `ORDER BY` invalidates existing parts and requires a table rebuild, so pick it against your real query mix before the first
deployment. The default puts the time bound first because every audit read is expected to carry one.

## Usage

```go
conn, err := clickhouse.Open(&clickhouse.Options{Addr: []string{"ch:9000"}})
if err != nil {
    return err
}
defer conn.Close()

storage, err := auditclickhouse.New(conn,
    auditclickhouse.WithTableName("audit_events"),
    auditclickhouse.WithTimeRangeMode(auditclickhouse.TimeRangeModeEnforce),
)
if err != nil {
    return err
}

if err := storage.StoreBatch(ctx, events); err != nil {
    return err
}

for event, err := range storage.Query(ctx, &audit.Query{StartTime: &from, ActorID: "user-42", Limit: 100}) {
    if err != nil {
        return err
    }
    fmt.Println(event.ID)
}
```

### Wiring into the dispatcher

No adapter is needed. `Storage.StoreBatch` already has the signature `dispatch.Sink` expects, and `data/audit` ships the wrapper and the
WAL codec for it:

```go
engine, err := dispatch.NewEngine(audit.StorageSink{Storage: storage},
    dispatch.WithWAL[*audit.Event](walDir, audit.JSONCodec{}))
if err != nil {
    return err
}
if err := engine.Start(); err != nil {
    return err
}

auditor, err := audit.New(engine)
```

[`data/audit/factory`](../../factory) does this assembly from configuration, including backend selection — reach for it before wiring the
chain by hand.

### Applying the schema from a migration

Instead of `WithAutoCreateTable`:

```go
ddl, err := auditclickhouse.SchemaDDL("audit_events", auditclickhouse.DefaultEngine, "", 400*24*time.Hour)
if err != nil {
    return err
}
if err := conn.Exec(ctx, ddl); err != nil {
    return err
}
```

## Behavior worth knowing

- **Feed it large batches.** Every INSERT creates a part the server must later merge, so ClickHouse wants thousands of rows per write, not
  hundreds. The `service/dispatch` defaults (`batchSize: 100`, `flushInterval: 1s`) are tuned for a row store; leaving them in place here
  means constant merge pressure and, on a busy service, `too many parts`. Aim for `batchSize` in the low thousands and a `flushInterval` of
  a few seconds — the audit trail is asynchronous anyway, so the added latency costs nothing. `data/audit/factory` warns when it sees a
  batch size below `MinRecommendedClickHouseBatchSize`.
- **`Store` versus `StoreBatch`.** `StoreBatch` is the hot path. `Store` handles the occasional lone event through an asynchronous insert,
  because a single-row INSERT would otherwise create a part of its own.
- **`WithMaxBatchSize` is a ceiling, not a target.** It splits oversized input so one call cannot become one enormous INSERT; it never
  merges small batches into bigger ones. Batch sizing is the dispatcher's job.
- **Duplicates are collapsed lazily.** The dispatcher delivers at-least-once; `ReplacingMergeTree` removes replayed rows by the sorting key,
  but only once the parts merge. `audit.FetchPage` returns each `(timestamp, id)` once regardless; raw `Query` and `Count` need
  `WithFinal`, which merges at query time, to stop observing a transient duplicate.
- **Batches are not atomic with respect to each other.** A failure partway through a split `StoreBatch` leaves the earlier batches inserted.
- **Timestamps keep milliseconds.** The column is `DateTime64(3, 'UTC')`; sub-millisecond detail is dropped on write and values read back are
  in UTC.
- **Empty containers normalize to nil.** The row cannot distinguish an empty map, slice, `Resource.Changes`, or `Result.Error` from an absent
  one, so all of them read back as nil.
- **Paging is keyset-based.** `audit.FetchPage` resolves the signed page token in `Query.After` and hands the storage the position in
  `Query.Cursor`, which becomes a tuple comparison `(timestamp, id) < (…, …)` in the WHERE clause — so the sorting key serves it instead of
  reading and discarding skipped rows. The sort is `(timestamp, id)` rather than `timestamp` alone because a batch of events shares a
  timestamp, and without the tiebreaker the position a cursor names would be ambiguous. Tokens are issued and verified by
  `audit.PageTokens`, bound to the query's filter, sort order and subject.
- **Time bounds bind as epoch milliseconds.** `StartTime`, `EndTime`, and the cursor render as `fromUnixTimestamp64Milli(?)`. Letting the
  driver render a `time.Time` loses sub-second precision, which is invisible on a coarse range filter but breaks a cursor: the bound has to
  compare exactly equal to the stored `DateTime64(3)` value, or the page is dropped entirely or replays the row it should have resumed past.
- **`ON CLUSTER` is not replication.** `WithCluster` only tells ClickHouse to run the DDL on every node of the cluster; the data is still
  local to whichever node received the INSERT. A replicated deployment needs both: `WithCluster("your_cluster")` **and** a `Replicated*`
  engine, for example `WithEngine("ReplicatedReplacingMergeTree('/clickhouse/tables/{shard}/audit_events', '{replica}')")`. Setting the
  cluster alone silently gives you unreplicated tables that merely exist everywhere.
- **Erasing one subject is a mutation.** `audit.Storage` has no delete method; `ALTER TABLE ... DELETE` applies asynchronously by rewriting
  parts, so plan retention around TTL and partitions instead.

## Testing

Unit tests run against a double and need no server. The integration tests are skipped unless a live server is configured:

```bash
CLICKHOUSE_ADDR=127.0.0.1:9000 go test -race ./data/audit/storages/clickhouse/
```

`CLICKHOUSE_DATABASE`, `CLICKHOUSE_USER`, and `CLICKHOUSE_PASSWORD` override the defaults. Each test owns a table named after itself and
drops it afterwards.

## See also

- [`data/audit`](../..) — the auditor and dispatcher that feed this storage.
- [`storages/memory`](../memory) — in-memory backend for dev and test.
- [`storages/mongo`](../mongo) — MongoDB backend.

## Redelivery

The dispatcher delivers at least once, so the same event can arrive twice. ClickHouse has no unique constraint, so nothing rejects the second
copy — the insert succeeds either way and `StoreBatch` returns no error.

Whether the duplicate survives depends on the engine:

- With the default `ReplacingMergeTree`, rows sharing the sorting key collapse to one — but only **when parts merge**, which happens on
  ClickHouse's schedule. A query issued before that returns both copies.
- With a plain `MergeTree`, both copies remain indefinitely.

So deduplication here is eventual at best. Paging through `audit.FetchPage` is unaffected: the copies share their `(timestamp, id)` key, sit
next to each other in the result and are collapsed into one, and a copy past a page boundary is skipped by the cursor. `Count` and raw `Query`
still see both copies until the merge, unless reads use `WithFinal` — paying for it on every query. This differs from the
[MongoDB storage](../mongo), which rejects the duplicate outright; [memory](../memory) keeps both copies, like a plain `MergeTree`.
