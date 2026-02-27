# metadata

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
```

Package `metadata` provides gRPC call metadata extraction and context injection. Creates `CallMetadata` once per RPC with service name, method
name, stream type, timing, client IP, and user agent. Parsed method names are cached process-wide via `sync.Map` for O(1) subsequent lookups.

## Key types

| Type / Interface | Description                                                                           |
|------------------|---------------------------------------------------------------------------------------|
| `CallMetadata`   | Immutable per-RPC info: ServiceName, MethodName, StreamType, StartTime, ClientPerIP   |

## CallMetadata fields

| Field             | Description                                                         |
|-------------------|---------------------------------------------------------------------|
| `StartTime`       | When the RPC began                                                  |
| `ServiceName`     | Extracted from method path (e.g., "UserService")                    |
| `MethodName`      | Extracted from method path (e.g., "GetUser")                        |
| `FullyMethodName` | Full gRPC method path (e.g., "/UserService/GetUser")                |
| `IsStream`        | Whether the RPC is streaming                                        |
| `IsClient`        | Whether metadata was captured on the client side                    |
| `StreamType`      | Streaming direction: None, Client, Server, Bidi                     |
| `ClientPerIP`     | Peer IP address (`netip.Addr`, server-side only)                    |
| `ClientUserAgent` | User-agent string from incoming gRPC metadata                       |

## Functions

| Function                  | Description                                                      |
|---------------------------|------------------------------------------------------------------|
| `NewCallMetadata`         | Creates CallMetadata from gRPC call information                  |
| `FromContext`             | Retrieves CallMetadata from context                              |
| `NewContext`              | Returns a new context with CallMetadata injected                 |
| `EnsureInContext`         | Get-or-create pattern for server interceptors                    |
| `EnsureInContextFromMethod`| Get-or-create pattern for client interceptors                  |
| `Interceptor`             | Returns a DrivenInterceptor auto-prepended by Chain              |
