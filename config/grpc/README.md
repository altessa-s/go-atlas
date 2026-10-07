# grpcconfig

```go
import grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
```

Package `grpcconfig` defines the gRPC server schema and the schemas of its interceptors. Schemas are populated by [`config/loader`](../loader) and
consumed by the component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type                               | Description                                                              |
|------------------------------------|--------------------------------------------------------------------------|
| `TLS`                          | Represents TLS configuration for gRPC servers.                           |
| `KeepAliveEnforcementPolicy`            | Represents keepalive enforcement policy for gRPC server.                 |
| `KeepAlive`                    | Represents keepalive parameters for gRPC server connections.             |
| `Config`                             | Represents the configuration for gRPC server settings.                   |
| `AuthInterceptor`              | Defines the configuration for authentication interceptor.                |
| `InterceptorFilter`          | Defines common method filtering settings used by interceptors.           |
| `BaseInterceptor`        | Provides common configuration structure for gRPC interceptors.           |
| `CacheCompressionPreset`  | Defines the compression levels available for cached responses.           |
| `CacheInterceptor`             | Defines the configuration for gRPC response caching.                     |
| `ErrStatusInterceptor`         | Defines the configuration for the error status interceptor.              |
| `GeoACLInterceptor`            | Defines the configuration for the geographic access control interceptor. |
| `HealthInterceptor`            | Defines the configuration for health check interceptor.                  |
| `IdempotencyInterceptor`       | Defines the configuration for idempotency interceptor.                   |
| `IPACLInterceptor`             | Defines the configuration for the IP access control interceptor.         |
| `LimiterInterceptor`           | Defines the configuration for rate limiting interceptor.                 |
| `LoggerInterceptor`            | Defines the configuration for request/response logging interceptor.      |
| `MetricsSamplingStrategy` | Defines how sampling is applied to streaming messages.                   |
| `MetricsInterceptor`           | Defines the configuration for the metrics interceptor.                   |
| `RealIPInterceptor`            | Defines the configuration for real IP extraction from proxy headers.     |
| `RecoveryInterceptor`          | Defines the configuration for panic recovery interceptor.                |
| `RequestIDInterceptor`         | Defines the configuration for request ID generation.                     |
| `TracingInterceptor`           | Defines the configuration for the tracing interceptor.                   |
| `Interceptors`               | Defines the configuration for gRPC interceptors.                         |

See the [config index](../README.md) for the other schema packages.
