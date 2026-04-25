# responder

```go
import "github.com/altessa-s/go-atlas/transport/http/server/responder"
```

Package `responder` provides unified error response handling for HTTP middleware and handlers. Enables middleware to write structured
error responses using the same writer + builder pattern as handlers, regardless of whether they call `http.Error` or the explicit
`WriteError` helper.

## Key types

| Type / Interface     | Description                                                                         |
|----------------------|-------------------------------------------------------------------------------------|
| `ErrorInterceptor`   | Wraps `http.ResponseWriter`, buffers error responses (status >= 400), rewrites as   |
|                      | structured JSON/XML via content negotiation when flushed                             |
| `ErrorWriter`        | Interface injected into context by the server's error interceptor middleware         |

## Functions

| Function     | Description                                                                                  |
|--------------|----------------------------------------------------------------------------------------------|
| `WriteError` | Retrieves `ErrorWriter` from context and writes a structured error, falls back to `http.Error` |
