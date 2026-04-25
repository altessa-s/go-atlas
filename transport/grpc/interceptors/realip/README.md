# realip

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/realip"
```

Package `realip` provides a gRPC server interceptor for extracting real client IP addresses from proxy headers. Processes X-Forwarded-For,
X-Real-IP, CF-Connecting-IP from gRPC metadata using a configurable `clientip.Extractor` with trusted proxy support. Injects IP into context.

## Options

| Option       | Default | Description                    |
|--------------|---------|--------------------------------|
| `WithLogger` | discard | Structured logger              |
