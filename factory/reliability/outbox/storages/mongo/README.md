# factory

```go
import outboxmongofactory "github.com/altessa-s/go-atlas/factory/reliability/outbox/storages/mongo"
```

Maps a MongoDB database or collection to a [MongoDB outbox storage](../../../../../reliability/outbox/storages/mongo/) for [`factory/reliability/outbox`](../..),
which builds only the SQL storage. The indexes are created on the build context, so a canceled startup fails the build instead of hanging on an
unreachable server.

## Usage

```go
ob, err := outboxfactory.New(&cfg.Outbox).
    UseScheduler(scheduler).
    BuildWithStorage(ctx, outboxmongofactory.Builder(db), handler)
```

| Symbol                   | Description                                                                |
|--------------------------|----------------------------------------------------------------------------|
| `Builder(db)`            | Storage function over the default collection of `db`; the caller owns `db` |
| `CollectionBuilder(col)` | Storage function over an existing collection; the caller owns `col`        |
| `ErrDatabaseRequired`    | Returned when `Builder` has no database                                    |
| `ErrCollectionRequired`  | Returned when `CollectionBuilder` has no collection                        |
