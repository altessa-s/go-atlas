# server

```go
import "github.com/altessa-s/go-atlas/transport/http/server"
```

Package `server` provides an HTTP server with graceful shutdown, middleware support, and pluggable router implementations. The router
abstraction decouples the server from any specific routing library, so implementations can be swapped without changing handler code.
Handlers receive a `writer.ReadWriter` for content-negotiated request/response handling instead of raw `http.ResponseWriter`.

## Subpackages

| Package                            | Description                                                              |
|------------------------------------|--------------------------------------------------------------------------|
| [codec](./codec)                   | Content-type codecs and registry with content negotiation                |
| [factory](./factory)               | Configuration-driven server and middleware creation                      |
| [handlers](./handlers)             | Common HTTP handlers (ping, healthz, pprof, metrics)                    |
| [middlewares](./middlewares)        | Middleware interfaces, chain, and dependency-based ordering              |
| [responder](./responder)           | Structured error response interception                                   |
| [router](./router)                 | Router interfaces and implementations (gorilla, std)                    |
| [writer](./writer)                 | Content-negotiated response writing with builder pattern                |
