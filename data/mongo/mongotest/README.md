# mongotest

```go
import "github.com/altessa-s/go-atlas/data/mongo/mongotest"
```

Env-gated MongoDB connection for integration tests of services built on [`data/mongo`](..). Tests skip when the URI variable is unset, so a plain
`go test` needs no infrastructure.

## API

| Function                   | Behavior                                                                                                         |
|----------------------------|------------------------------------------------------------------------------------------------------------------|
| `New(tb, opts...)`         | Connected `*datamongo.Mongo` on a throwaway database `<prefix><8 hex>`; a cleanup drops it and closes the client |
| `IndexNames(tb, m, coll)`  | Sorted index names of a collection, including `_id_` (none for a missing collection)                             |
| `RequireReplicaSet(tb, m)` | Fails the test unless the server is a replica set member or a mongos (transactions, change streams)              |

`New` skips the test when the variable is unset or empty and fails it when a configured MongoDB cannot be used. The client uses the go-atlas codec
registry like every `datamongo.Mongo`, and `Connect` runs the registered migrations.

## Options

| Option               | Default | Description                                       |
|----------------------|---------|---------------------------------------------------|
| `WithEnv`            | `MONGO_URI` | Environment variable holding the connection URI |
| `WithDatabasePrefix` | `it_`   | Start of the throwaway database name              |

```go
m := mongotest.New(t, mongotest.WithEnv("APP_IT_MONGO_URI"), mongotest.WithDatabasePrefix("app_it_"))
mongotest.RequireReplicaSet(t, m)
require.Contains(t, mongotest.IndexNames(t, m, "orders"), "idx_customer")
```
