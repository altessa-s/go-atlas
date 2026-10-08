# recovery

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/recovery"
```

Package `recovery` provides middleware that recovers from panics in HTTP handlers, logs the panic with a stack trace, and returns a 500
Internal Server Error response. Panics with `http.ErrAbortHandler` are re-raised rather than recovered, matching the standard library
convention. Supports a custom `PanicHandler` for application-specific error responses. Declares a dependency on `requestid` for log
correlation.

## Started responses

A panic can come after the handler has begun the response. `HeadersWritten(w)` reports, from inside a `PanicHandler`, whether the status and
headers are already on the wire (`WriteHeader`, `Write`, `Flush` or `Hijack`); 1xx informational responses do not count. A handler that sees
`true` should only log, or abort the connection by panicking with `http.ErrAbortHandler`. The default handler writes the 500 only when the
response has not started.

```go
recovery.WithHandler(func(ctx context.Context, w http.ResponseWriter, r *http.Request, p any) {
    if recovery.HeadersWritten(w) {
        return
    }
    renderErrorPage(w, http.StatusInternalServerError)
})
```
