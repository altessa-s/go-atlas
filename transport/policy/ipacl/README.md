# ipacl

```go
import "github.com/altessa-s/go-atlas/transport/policy/ipacl"
```

Package ipacl provides IP-based access control logic shared by the gRPC interceptor and HTTP middleware layers.  A [Registry] holds per-endpoint and pattern-based [AccessRule] entries.

The HTTP middlewares and gRPC interceptors use it; the server factories build it from configuration.

## API

`ParsePolicy`, `Policy`, `AccessRule`, `Registry`, `NewRegistry`, `Evaluate` — see the package documentation.
