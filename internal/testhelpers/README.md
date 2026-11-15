# testhelpers

```go
import "github.com/altessa-s/go-atlas/internal/testhelpers"
```

Package `testhelpers` provides shared fixtures and doubles for tests across toolkit packages. Helpers accepting `testing.TB` register cleanup
automatically.

## Key helpers

| Helper                                | Purpose                                                                                          |
|---------------------------------------|--------------------------------------------------------------------------------------------------|
| `MockTaskRegistrar`                   | Captures scheduler registrations and injected registration errors                                |
| `NewCA`, `SelfSignedCert`             | TLS fixtures                                                                                     |
| `NewFakeSQL`                          | Scripted `database/sql` driver recording statements for SQL storage tests                        |
| `ApplyLeafPaths`                      | Copies update-mask leaf paths between protobuf messages, as a storage update by leaf paths would |
| `WaitFor`                             | Bounded asynchronous assertions                                                                  |

Capability fixtures live with their capability: [`audittest`](../../audit/audittest), [`idempotencytest`](../../reliability/idempotency/idempotencytest),
[`metricstest`](../../observability/metrics/metricstest); in-process servers in [`servers/natstest`](servers/natstest) and
[`servers/redistest`](servers/redistest). See [doc.go](doc.go) for the full helper inventory. Keep fixtures used only within one package tree in that tree's internal test helpers.
