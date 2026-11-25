# recovery

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/recovery"
```

Package `recovery` provides middleware that recovers from panics in HTTP handlers, logs a fixed panic category and the route template, and returns a 500 Internal Server
Error response. Panics with `http.ErrAbortHandler` are re-raised rather than recovered, matching the standard library convention. Supports a custom
`PanicHandler` for application-specific error responses. It requests outermost placement in ordered chains and logs the ID resolved by
`requestid`, including generated IDs.

## Options

| Option               | Default                | Description                                         |
|----------------------|------------------------|-----------------------------------------------------|
| `WithHandler`        | bare 500               | `PanicHandler` that writes the error response       |
| `WithLogRawErrors`   | off                    | Log the raw panic value and stack trace             |
| `WithIgnorePaths`    | --                     | Paths to skip                                       |
| `WithIgnorePatterns` | --                     | Regex patterns for paths to skip                    |
| `WithLogger`         | discard                | Structured logger                                   |

Recovery covers every path by default, including health, metrics, and pprof endpoints. The route is `/_unknown` when no template is available.
Enable `WithLogRawErrors(true)` (YAML `logRawErrors: true`) to include raw diagnostics.

## Started responses

A panic can come after the handler has begun the response. `HeadersWritten(w)` reports, from inside a `PanicHandler`, whether the status and headers are
already on the wire (`WriteHeader`, `Write`, `Flush` or `Hijack`); 1xx informational responses other than 101 Switching Protocols do not count. A
handler that sees `true` should only log, or abort the connection by panicking with `http.ErrAbortHandler`. The default handler writes the 500 only when
the response has not started.

```go
recovery.WithHandler(func(ctx context.Context, w http.ResponseWriter, r *http.Request, p any) {
    if recovery.HeadersWritten(w) {
        return
    }
    renderErrorPage(w, http.StatusInternalServerError)
})
```
