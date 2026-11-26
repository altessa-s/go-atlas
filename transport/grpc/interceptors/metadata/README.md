# metadata

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
```

Package `metadata` provides gRPC call metadata extraction and context injection. Creates `CallMetadata` once per RPC with service name, method name,
stream type, timing, client IP, and user agent. Registered server method names are cached process-wide in a bounded `sync.Map` for O(1) subsequent
lookups; unknown and client method names are parsed without interning.

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
| `NewCallMetadataFromMethod`| Creates CallMetadata from the full method name only             |
| `FromContext`             | Retrieves CallMetadata from context                              |
| `MethodFromContext`       | Returns the full method name stored in context, or empty         |
| `NewContext`              | Returns a new context with CallMetadata injected                 |
| `EnsureInContext`         | Get-or-create pattern for server interceptors                    |
| `EnsureInContextFromMethod`| Get-or-create pattern for client interceptors                  |
| `Interceptor`             | Returns a DrivenInterceptor auto-prepended by Chain              |
| `ServerMethodOptions`     | Installs the serving gRPC server's registered method set          |

## Unknown methods

Atlas's gRPC server installs the registered method set automatically. With a standalone `grpc.Server`, pass `ServerMethodOptions` a callback
to the serving server's `GetServiceInfo` and add those options before other chained interceptors. Without a method set, metadata creation
does not admit methods to the cache.

The standard stream interceptor uses `/_unknown/Unknown` for calls dispatched through a custom gRPC unknown-service handler. The handler can
read the original path via `grpc.Method`.
