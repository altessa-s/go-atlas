# lifecycle

```go
import "github.com/altessa-s/go-atlas/transport/lifecycle"
```

Both the [HTTP server](../http/server) and the [gRPC server](../grpc/server) accept a `Timeouts` value through their `WithLifecycleTimeouts` option.

## Defaults

| Field                 | Default |
|-----------------------|---------|
| `StartupVerification` | 2s      |
| `ShutdownGraceful`    | 1m      |
| `ShutdownWarning`     | 15s     |

Use `DefaultTimeouts()` to override individual defaults. A zero startup window skips verification. Nonpositive shutdown durations select their defaults.
