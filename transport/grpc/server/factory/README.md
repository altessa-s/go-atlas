# factory

```go
import "github.com/altessa-s/go-atlas/transport/grpc/server/factory"
```

Package `factory` provides configuration-based creation of gRPC servers and interceptors. Use `New` to create a `Factory`, then call the Create*
methods to build server components from config structs. All created components inherit the factory's logger. When TLS is required the factory
resolves certificates through configured `tlsproviders.Providers`. Each interceptor method returns `(nil, nil)` when the configuration is nil or
disabled, allowing callers to pass the result directly to `interceptors.ServerConditionalInterceptor`.

## Factory options

| Option                       | Default | Description                                                          |
|------------------------------|---------|----------------------------------------------------------------------|
| `WithLogger`                 | discard | Structured logger inherited by all created components                |
| `WithTlsProviders`          | nil     | TLS certificate providers used when server TLS config is present     |
| `WithCacheMetadataProcessor` | nil     | Custom metadata processor forwarded to the cache interceptor         |

## Methods

| Method                                       | Description                                                        |
|----------------------------------------------|--------------------------------------------------------------------|
| `CreateServerFromConfig`                     | Builds a gRPC server with TLS, keepalive, and reflection settings  |
| `CreateLoggerInterceptorFromConfig`          | Structured request/response logging interceptor                    |
| `CreatePrometheusInterceptorFromConfig`      | Prometheus metrics collection interceptor                          |
| `CreateTracingInterceptorFromConfig`         | Distributed tracing interceptor using a provided `Tracer`          |
| `CreateRealIPInterceptorFromConfig`          | Extracts the real client IP from proxy headers                     |
| `CreateRecoveryInterceptorFromConfig`        | Panic recovery interceptor that returns `codes.Internal`           |
| `CreateRequestIDInterceptorFromConfig`       | Propagates or generates a request ID for every RPC                 |
| `CreateLimiterInterceptorFromInterConfig`    | Rate limiting interceptor backed by a `Limiter` implementation     |
| `CreateIdempotencyInterceptorFromInterConfig`| Idempotency enforcement interceptor backed by a `Keeper`           |
| `CreateCacheInterceptorFromConfig`           | Response caching interceptor with compression and TTL              |
| `CreateAuthInterceptorFromConfig`            | Authentication interceptor with token validation and client auth   |
| `CreateHealthInterceptorFromConfig`          | Health gate interceptor that rejects traffic when unhealthy        |
