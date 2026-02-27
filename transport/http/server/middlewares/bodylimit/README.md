# bodylimit

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/bodylimit"
```

Package `bodylimit` provides middleware that rejects requests whose body exceeds a configured size limit. The middleware performs two
checks: a fast pre-flight Content-Length header check that returns 413 immediately, and a streaming enforcement via `http.MaxBytesReader`
to catch chunked or misreported bodies. Error responses use the `responder.WriteError` helper for content-negotiated structured output.
