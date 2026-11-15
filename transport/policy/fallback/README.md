# fallback

```go
import "github.com/altessa-s/go-atlas/transport/policy/fallback"
```

Package fallback defines the three strategies — allow, deny, error — that middlewares and interceptors apply when a check fails with an error; unrecognized values parse to deny.

The HTTP middlewares and gRPC interceptors use it; the server factories build it from configuration.

## API

`Behavior`, `String`, `ShouldAllow`, `ShouldDeny`, `ShouldReturnError`, `IsValid`, `MustValidate`, `ParseBehavior` — see the package documentation.
