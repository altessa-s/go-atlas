# requestid

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/requestid"
```

Package `requestid` provides middleware for UUID v4 request ID extraction or generation. The request ID is read from the incoming request
header (configurable, defaults to X-Request-Id). If the header is missing or contains an invalid UUID and generation is enabled, a new
UUID v4 is generated. The ID is stored in the request context (retrievable via `FromContext`) and echoed in the response header. OPTIONS
requests are always passed through without processing.
