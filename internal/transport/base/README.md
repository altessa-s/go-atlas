# base

```go
import "github.com/altessa-s/go-atlas/internal/transport/base"
```

Shared endpoint filtering and structured logging for HTTP middleware and gRPC interceptors.

## Key functions

| Function | Purpose |
|----------|---------|
| `NewWithFilter` | Creates a base with endpoint exclusions. |
| `LogWarn / LogError` | Attach a bounded diagnostic to a record with component and endpoint fields. |
