# tracing

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/tracing"
```

Package `tracing` provides HTTP middleware for distributed tracing. Integrates with the `observability/tracing` package for automatic span creation on
each HTTP request, W3C Trace Context propagation via HTTP headers, and error status for 4xx and 5xx responses. Default span names
combine the HTTP method and matched route template, using `/_unknown` for misses.

## Span attributes

| Attribute          | Description                                                                          |
|--------------------|--------------------------------------------------------------------------------------|
| `http.route`       | Matched route template or `/_unknown`                                                |
| `http.method`      | Request method (GET, POST, etc.)                                                     |
| `http.scheme`      | Request scheme (http/https)                                                          |
| `http.status_code` | Response status code                                                                 |

`WithRecordClientDetails(true)` (YAML `recordClientDetails: true`) adds host, user agent, client IP and the raw path and URL.
The URL omits the query. `WithRecordRawErrors(true)` (YAML `recordRawErrors: true`) records a panic value and stack trace when
a panic unwinds through tracing.
