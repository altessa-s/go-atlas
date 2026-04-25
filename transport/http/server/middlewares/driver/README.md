# driver

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/driver"
```

Package `driver` provides the Driven Middleware pattern for HTTP middlewares. Separates lifecycle hooks (PreRequest/PostRequest) from the
HTTP handler mechanics, making it easier to implement middlewares that need pre/post processing. This pattern mirrors the gRPC interceptor
driver pattern for consistency across transports.

## Key types

| Type / Interface      | Description                                                                        |
|-----------------------|------------------------------------------------------------------------------------|
| `Driver`              | Interface: `PreRequest(ctx, r)` and `PostRequest(ctx, w, r, err)` lifecycle hooks  |
| `DrivenMiddleware`    | Interface: creates a request-scoped `Driver` for each incoming request             |
| `HTTPDrivenMiddleware`| Wraps a `DrivenMiddleware` into a standard `middlewares.Middleware`                 |
