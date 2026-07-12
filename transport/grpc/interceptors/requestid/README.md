# requestid

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/requestid"
```

Package `requestid` provides gRPC interceptors for request ID generation and propagation. UUID v4 identifiers extracted from incoming metadata or
auto-generated. Server-side extraction with validation, client-side propagation to outgoing metadata.

## Key types

| Type / Interface      | Description                                                            |
|-----------------------|------------------------------------------------------------------------|
| `ErrInvalidRequestId` | Sentinel error for invalid request IDs (not UUID v4)                   |

## Functions

| Function              | Description                                                    |
|-----------------------|----------------------------------------------------------------|
| `ServerInterceptor`   | Server-side: extracts or generates request ID from metadata    |
| `ClientInterceptor`   | Client-side: propagates request ID to outgoing metadata        |
| `FromContext`         | Retrieves request ID from context                              |
| `NewContext`          | Returns a new context with the given request ID                |
