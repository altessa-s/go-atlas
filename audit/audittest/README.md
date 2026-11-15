# audittest

```go
import "github.com/altessa-s/go-atlas/audit/audittest"
```

Test kit for [`audit`](../): it an in-memory audit.Auditor for tests of packages that emit audit events (HTTP and gRPC audit middleware, auth sinks).

## Key types

| Symbol                        | Description                                                                                             |
|-------------------------------|---------------------------------------------------------------------------------------------------------|
| `NewTestAuditor(tb, opts...)` | Auditor over an in-memory storage; returns the storage and a shutdown func registered with `tb.Cleanup` |
| `ShutdownFunc`                | Shutdown signature returned by `NewTestAuditor`                                                         |
