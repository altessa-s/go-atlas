# mongotest

```go
import "github.com/altessa-s/go-atlas/store/mongo/mongotest"
```

Env-gated MongoDB connection for integration tests of services built on [`store/mongo`](..). Tests skip when the URI variable is unset, so a plain
`go test` needs no infrastructure.

## API

| Function                   | Behavior                                                                                                         |
|----------------------------|------------------------------------------------------------------------------------------------------------------|
| `New(tb, opts...)`         | Connected `*datamongo.Mongo` on a throwaway database `<prefix><created>_<8 hex>`; a cleanup drops it and closes the client |
| `IndexNames(tb, m, coll)`  | Sorted index names of a collection, including `_id_` (none for a missing collection)                             |
| `RequireReplicaSet(tb, m)` | Fails the test unless the server is a replica set member or a mongos (transactions, change streams)              |

`New` skips the test when the variable is unset or empty and fails it when a configured MongoDB cannot be used. The client uses the go-atlas codec
registry like every `datamongo.Mongo`, and `Connect` runs the registered migrations.

Every test gets its own client. Before `Connect`, whose single ping has a fixed 5s timeout and does not retry a server selection timeout, `New` waits
up to the readiness timeout for the new client's first server handshake: a loaded integration suite can delay it past that ping, which used to fail the
test with `server selection error: context deadline exceeded`. The client's server selection timeout is raised to the same budget, so the test's own
operations also wait for the server instead of failing early. A server that never answers within the budget still fails the test.

`<created>` is the creation time as 8 hex digits of Unix seconds. A run killed before its cleanups (an interrupted `go test`, a test timeout) leaves
its databases behind, so the first `New` of the process for a URI and prefix drops every database of that prefix created more than the stale age ago.
The age must outlast the longest run sharing the server, so a concurrent run keeps its databases. Names without a creation time and other prefixes are
never touched; the sweep is best effort and only logs a failure.

## Options

| Option                 | Default     | Description                                                        |
|------------------------|-------------|--------------------------------------------------------------------|
| `WithEnv`              | `MONGO_URI` | Environment variable holding the connection URI                    |
| `WithDatabasePrefix`   | `it_`       | Start of the throwaway database name                               |
| `WithReadinessTimeout` | `2m`        | Wait for the first server handshake (and client server selection) |
| `WithStaleAge`         | `2h`        | Age past which leftover databases of the prefix are dropped; `0` disables the sweep |

```go
m := mongotest.New(t, mongotest.WithEnv("APP_IT_MONGO_URI"), mongotest.WithDatabasePrefix("app_it_"))
mongotest.RequireReplicaSet(t, m)
require.Contains(t, mongotest.IndexNames(t, m, "orders"), "idx_customer")
```
