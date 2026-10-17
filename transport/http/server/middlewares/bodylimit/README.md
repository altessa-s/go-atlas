# bodylimit

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/bodylimit"
```

Package `bodylimit` provides middleware that rejects requests whose body exceeds a configured size limit. The middleware performs two checks: a fast
pre-flight Content-Length header check that returns 413 immediately, and a streaming enforcement via `http.MaxBytesReader` to catch chunked or
misreported bodies. Error responses use the `responder.WriteError` helper for content-negotiated structured output.

## Options

| Option                     | Default          | Description                                                                         |
|----------------------------|------------------|-------------------------------------------------------------------------------------|
| `WithRequireContentLength` | off              | Reject POST/PUT/PATCH/DELETE without a declared Content-Length with 411             |
| `WithLimitFunc`            | static limit     | Per-request limit: positive bytes, `0` for the static limit, negative for no limit  |
| `WithErrorWriter`          | context / text   | `responder.ErrorWriter` for the 413 / 411 responses, e.g. an HTML error page        |

## Usage

```go
mw := bodylimit.New(1<<20, // 1 MiB by default
    bodylimit.WithLimitFunc(func(r *http.Request) int64 {
        if strings.HasPrefix(r.URL.Path, "/upload/") {
            return 64 << 20
        }
        return 0
    }),
    bodylimit.WithErrorWriter(htmlErrors),
)
```
