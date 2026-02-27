# recovery

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/recovery"
```

Package `recovery` provides middleware that recovers from panics in HTTP handlers, logs the panic with a stack trace, and returns a 500
Internal Server Error response. Panics with `http.ErrAbortHandler` are re-raised rather than recovered, matching the standard library
convention. Supports a custom `PanicHandler` for application-specific error responses. Declares a dependency on `requestid` for log
correlation.
