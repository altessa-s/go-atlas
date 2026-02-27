# tracing

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/tracing"
```

Package `tracing` provides HTTP middleware for distributed tracing. Integrates with the `observability/tracing` package for automatic
span creation on each HTTP request, W3C Trace Context propagation via HTTP headers, and error recording for 4xx and 5xx responses.

## Span attributes

| Attribute          | Description                                                                          |
|--------------------|--------------------------------------------------------------------------------------|
| `http.method`      | Request method (GET, POST, etc.)                                                     |
| `http.url`         | Full request URL                                                                     |
| `http.target`      | Request path                                                                         |
| `http.host`        | Request host                                                                         |
| `http.scheme`      | Request scheme (http/https)                                                          |
| `http.status_code` | Response status code                                                                 |
| `http.user_agent`  | User agent string                                                                    |
| `net.peer.ip`      | Client IP address                                                                    |
