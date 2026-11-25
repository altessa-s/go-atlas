# server

```go
import "github.com/altessa-s/go-atlas/transport/http/server"
```

Package `server` provides an HTTP server with graceful shutdown, middleware support, and pluggable router implementations. The router abstraction
decouples the server from any specific routing library, so implementations can be swapped without changing handler code. Handlers receive a
`writer.ReadWriter` for content-negotiated request/response handling instead of raw `http.ResponseWriter`.

## Subpackages

| Package                            | Description                                                              |
|------------------------------------|--------------------------------------------------------------------------|
| [codec](./codec)                   | Content-type codecs and registry with content negotiation                |
| [factory](../../../factory/transport/http/server)               | Configuration-driven server and middleware creation                      |
| [handlers](./handlers)             | Common HTTP handlers (ping, healthz, pprof, metrics)                    |
| [middlewares](./middlewares)        | Middleware interfaces, chain, and dependency-based ordering              |
| [responder](./responder)           | Structured error response interception                                   |
| [router](./router)                 | Router interfaces and the `http.ServeMux` implementation (std)          |
| [writer](./writer)                 | Content-negotiated response writing with builder pattern                |

Use `WithAddress` or `WithListener` to choose the listening endpoint, and `WithName` and `WithLogger` to configure lifecycle logs. `Handle` owns cleanup
of the request helper, including when a handler panics.

## TLS listeners

With server TLS configured with `WithTLSConfig`, pass a raw listener if
you supply one. The protocol server negotiates TLS for both supplied and
automatically created listeners.

## Lifecycle configuration

Pass [`lifecycle.Timeouts`](../../lifecycle) to `WithLifecycleTimeouts`. Start with `lifecycle.DefaultTimeouts()` to override individual fields.
`LifecycleTimeouts()` returns the effective settings. The shorter of the graceful shutdown limit and the caller's deadline bounds protocol cleanup.
A second `Start` on a running server returns `ErrServerAlreadyStarted`. Both lifecycle errors support `errors.Is`.

## Lifecycle observation

`Done() <-chan struct{}` closes when shutdown is observed or a post-start Serve
failure occurs. After it closes, `Err() error` reports the first unexpected Serve
error, or nil for normal shutdown. Call `Shutdown` to finish protocol cleanup even
after a Serve failure.

Once `Done` closes, `Start` returns `ErrServerClosed`, including after a post-start
Serve failure. Create a new server to restart. Early startup errors are returned
by `Start` and leave `Done` open; a supplied listener may be consumed by a failed
Serve, so retrying with that listener may not succeed.

`WithRouteRegistrar` directs endpoint registrations to a subrouter when configured. The router supplied to `WithRouter` serves requests,
installs server middleware, and is initialized at startup. `OPTIONS *` passes through the router and its middleware. Standard-library
server diagnostics go to the server logger as a fixed error message that omits peer addresses and request data.
