# realip

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/realip"
```

Package `realip` provides a gRPC server interceptor for extracting real client IP addresses from proxy headers. Processes X-Forwarded-For and any
opted-in single-address header (X-Real-IP, CF-Connecting-IP, ...) from gRPC metadata using a configurable [`clientip.Extractor`](https://github.com/altessa-s/go-atlas-net/tree/develop/clientip) with
trusted proxy support. Injects IP into context.

## Options

| Option       | Default | Description                    |
|--------------|---------|--------------------------------|
| `WithLogger` | discard | Structured logger              |

## Diagnostics

Diagnostic logging is discarded by default. With a configured logger, `WithLogClientDetails(true)` permits logging peer addresses. Raw parser diagnostics require `WithLogRawErrors(true)` as well. The factory enables the external extractor logger only when both options are set.
