# requestid

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/requestid"
```

Package `requestid` provides gRPC interceptors for request ID generation and propagation. UUID v4 identifiers extracted from incoming metadata or
auto-generated. Server-side extraction with validation, client-side propagation to outgoing metadata.

## Trust model

A client-supplied metadata value that passes strict UUID v4 validation is trusted as-is: it is propagated, stored in the context, and
used as a log correlation field, so a hostile client chooses which UUID appears in logs and can reuse one across requests to spoof
correlation. Values that fail validation never propagate, so arbitrary client bytes cannot reach logs through this metadata key. There
is no option to ignore a valid inbound value — strip or replace the metadata at the edge proxy when server-authoritative IDs are
required.

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
