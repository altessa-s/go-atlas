# retryconfig

```go
import retryconfig "github.com/altessa-s/go-atlas/config/retry"
```

Package `retryconfig` defines the retry policy schema used by outbound clients. Schemas are populated by [`config/loader`](../loader) and consumed by
the component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type    | Description                                                               |
|---------|---------------------------------------------------------------------------|
| `Config` | Configures exponential backoff retry behavior for external service calls. |

See the [config index](../README.md) for the other schema packages.
