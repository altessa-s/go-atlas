# mongoit

```go
import "github.com/altessa-s/go-atlas/tests/integration/mongoit"
```

Integration tests for `store/mongo` against a live MongoDB replica set. The suite covers transactions, migration isolation and rollback, keyset pagination,
cursor security and unit-of-work behavior; fixtures create randomly named databases and drop them on cleanup.

## Coverage

| Area         | Contract                                                      |
|--------------|---------------------------------------------------------------|
| Transactions | Panics and failed units of work cannot commit partial writes   |
| Migrations   | Concurrent fixtures use distinct databases; rollback is opt-in |
| Pagination   | Keyset and cursor queries preserve query and identity bindings |
| Read filters | Monitored find, match and count predicates match the prepared BSON under inherited encoding settings |
| Projections  | Cursor pages under a projection keep the cursor sort key       |

## Running

Start the backends and run `make test-integration` from the repository root. For a focused run, use
`go test -C tests/integration -race ./mongoit`. See the [integration guide](../README.md) for connection settings and backend lifecycle.
