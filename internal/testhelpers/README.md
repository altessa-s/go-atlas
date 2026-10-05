# testhelpers

```go
import "github.com/altessa-s/go-atlas/internal/testhelpers"
```

Package `testhelpers` provides shared fixtures and doubles for tests across toolkit packages. Helpers accepting `testing.TB` register cleanup
automatically.

## Key helpers

| Helper | Purpose |
|--------|---------|
| `MockIdempotencyStorage` | In-memory lock storage with ownership checks for completion and release |
| `MockTaskRegistrar` | Captures scheduler registrations and injected registration errors |
| `RedisClient` | In-process Redis fixture |
| `StartNATSServer`, `ConnectJetStream` | Embedded NATS and JetStream fixtures |
| `NewCA`, `SelfSignedCert` | TLS fixtures |
| `NewTestCollector`, `GatherMetric` | Metrics assertions |
| `NewFakeSQL` | Scripted `database/sql` driver recording statements for SQL storage tests |
| `WaitFor` | Bounded asynchronous assertions |

See [doc.go](doc.go) for the full helper inventory. Keep fixtures used only within one package tree in that tree's internal test helpers.
