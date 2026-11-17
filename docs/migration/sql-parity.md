# Migrating to the SQL-parity APIs

Every durable backend that had only a MongoDB (or NATS/Redis) storage now also has a `sqldb` one, and the existing SQL backends were brought to one
shape. Two parts of that work change public APIs: the outbox is now built from configuration instead of from backend-specific factory methods, and the
retention sweeps of the SQL backends share one purge contract. This note lists every change that needs code or YAML edits. See
[configuration](../configuration.md#sql-backends) for the shared rules of the SQL backends.

## Outbox factory

`factory/reliability/outbox` selects the storage from `outbox.storage.type` instead of from the method you call. The positional build methods are
gone; dependencies are injected with setters and `Build` builds whatever the configuration names.

| Before                                                  | After                                                                                         |
|---------------------------------------------------------|-----------------------------------------------------------------------------------------------|
| `BuildWithStorage(ctx, outboxmongofactory.Builder(db), handler)` | `UseStorage(brokerconfig.OutboxStorageTypeMongo, outboxmongofactory.Builder(db)).Build(ctx, handler)` |
| `BuildWithSQLDB(ctx, db, dialect, handler)`             | `UseSQLDB(db).Build(ctx, handler)`, with the dialect in `outbox.storage.sqldb.dialect`        |
| —                                                       | `UseSQLListener(l)`: the PostgreSQL listener for `outbox.watch: notify`                       |
| —                                                       | `UseShutdownHooks(g)`: the hook group that stops the watcher the builder starts               |

```go
// Before
ob, err := outboxfactory.New(&cfg.Outbox).
    UseScheduler(scheduler).
    BuildWithSQLDB(ctx, db, outboxsql.DialectPostgres, handler)

// After
ob, err := outboxfactory.New(&cfg.Outbox).
    UseScheduler(scheduler).
    UseSQLDB(db).
    Build(ctx, handler)
```

A missing dependency fails `Build` with `factory/core.ErrDependencyRequired` (`UseSQLDB` for `sqldb`, `UseSQLListener` for `watch: notify`); a
`mongo` storage without `UseStorage` fails with `ErrNotRegistered`. `outboxmongofactory.Builder(db)` now uses the collection named by
`outbox.storage.mongo.collection`; `CollectionBuilder(col)` still uses the collection it is given.

A disabled outbox (`outbox.enabled: false`) now returns `(nil, nil)` before validating the storage or building anything. Before, the store was built
first, so a disabled outbox could still fail on a missing dependency or run index creation.

## Broker factory

`factory/messaging/broker` delegates to the outbox factory the same way.

| Before                                               | After                                                                      |
|------------------------------------------------------|----------------------------------------------------------------------------|
| `CreateOutboxWithStorage(ctx, build, publisher)`     | `UseOutboxStorage(brokerconfig.OutboxStorageTypeMongo, build).CreateOutbox(ctx, publisher)` |
| `CreateOutboxWithSQLDB(ctx, db, dialect, publisher)` | `UseSQLDB(db).CreateOutbox(ctx, publisher)`                                |
| —                                                    | `UseSQLListener(l)`, `UseShutdownHooks(g)`, as in the outbox factory       |

```go
// Before
b := brokerfactory.New(&cfg.Broker).UseScheduler(scheduler)
ob, err := b.CreateOutboxWithStorage(ctx, outboxmongofactory.Builder(mongoDB), provider)

// After
b := brokerfactory.New(&cfg.Broker).
    UseScheduler(scheduler).
    UseOutboxStorage(brokerconfig.OutboxStorageTypeMongo, outboxmongofactory.Builder(mongoDB))
ob, err := b.CreateOutbox(ctx, provider)
```

## Outbox configuration

The outbox block gains a storage selector and a watch mode; `ensureSchema` moved into the SQL block.

```yaml
# Before
outbox:
  ensureSchema: true

# After
outbox:
  storage:
    type: sqldb            # mongo (default) | sqldb
    mongo:
      collection: events_outbox
    sqldb:
      dialect: postgres    # postgres | mysql (MySQL 8.0+, MariaDB 10.6+)
      table: events_outbox
      ensureSchema: true
  watch: polling           # polling (default) | changeStream (mongo) | notify (sqldb, postgres)
```

A missing `storage` block is normalized to MongoDB by `Outbox.Normalize` (the factories call it). A watch mode the storage cannot serve —
`changeStream` on `sqldb`, `notify` on `mongo` or on `dialect: mysql` — fails validation with `brokerconfig.ErrUnsupportedOutboxWatch`. With
`changeStream` or `notify` the factory starts `outbox.Outbox.Watch` itself; drop any `go ob.Watch(ctx)` of your own. The dispatch schedule keeps
running in every mode.

## Purge contract of the SQL backends

The SQL backends of audit, dlock, idempotency and retryqueue delete what their retention expired through one method, configured the same way.

| Package                                 | Before                                   | After                                        |
|-----------------------------------------|------------------------------------------|----------------------------------------------|
| `coord/dlock/providers/sqldb`           | `Locker.DeleteExpired(ctx) (int64, error)` | `Locker.PurgeExpired(ctx) (int64, error)`    |
| `reliability/idempotency/storages/sqldb` | `DeleteExpired(ctx, limit) (int, error)` | `PurgeExpired(ctx) (int64, error)`           |
| `reliability/retryqueue/storages/sqldb` | `PurgeResolved(ctx) (int, error)`        | `PurgeExpired(ctx) (int64, error)`           |
| `audit/storages/sqldb`                  | `PurgeExpired(ctx) (int64, error)`       | unchanged                                    |

`PurgeExpired` deletes everything currently purgeable in batches of `WithPurgeBatchSize` (default `DefaultPurgeBatchSize`, 1000) and returns the
total, including the batches deleted before an error. Callers of the idempotency `limit` argument drop it:

```go
// Before
n, err := store.DeleteExpired(ctx, 500)

// After
n, err := store.PurgeExpired(ctx) // batches of WithPurgeBatchSize
```

Retention stays a package option — audit `WithTTL`, dlock `WithExpiredCleanup(grace)`, retryqueue `WithResolvedRetention`; idempotency keys carry
their own expiry. The background loop is `WithPurgeInterval(d)` everywhere (default `DefaultPurgeInterval`, one minute; `0` disables it) and runs only
when retention is set. It starts on the first write, not in `New`, and stops with `Close(ctx)`. The idempotency `WithExpiredCleanup(interval)` option
is now `WithPurgeInterval`.

Every `sqldb` block of audit, lock, idempotency and retryqueue takes the same two keys:

```yaml
# Before (idempotency)
storage:
  sqldb:
    expiredCleanupInterval: 1m

# After (idempotency, and the same keys for audit, lock and retryqueue)
storage:
  sqldb:
    purgeInterval: 1m      # "0s" disables the background loop
    purgeBatchSize: 1000
```

The retryqueue `purgeInterval` now defaults to `1m` and no longer fails validation without `resolvedRetention`; it is simply inactive. The factories
register the store's `Close` with their shutdown hooks whenever a loop can run.
