# client

```go
import "github.com/altessa-s/go-atlas/transport/grpc/client"
```

Package `client` provides a generic gRPC client with connection management, retry, health checking, and error handling. It is designed as a foundation
for building service-specific gRPC clients with consistent behavior for connection pooling, automatic retry with exponential backoff, structured
logging, and rich error parsing with field-level validation details.

## Key types

| Type / Interface  | Description                                                                              |
|-------------------|------------------------------------------------------------------------------------------|
| `Client`          | Core gRPC client: single connection or pool mode, retry, timeouts, health checking       |
| `Error`           | Structured gRPC error with code, request ID, reason, metadata, and field validations     |
| `FieldError`      | Single field violation from an `io.altessa.badrequest.v1.BadRequest` (go-atlas servers, with `Code`) or `google.rpc.BadRequest` detail |
| `RetryOption`     | Retry policy setters for `WithRetry`: max attempts, initial/max backoff, multiplier, status codes |
| `ErrorConverter`  | Hook to override default `ParseStatusError` behavior in the error-status interceptor     |
| `StaticToken`     | Per-RPC credentials with a fixed bearer token, created via `NewStaticToken`              |
| `InsecureTokenCredentials` | Per-RPC credentials from an `oauth2.TokenSource` without requiring TLS          |
| `ParseError` / `AsClientError` | `ParseError` converts a gRPC status error into `*Error`; `AsClientError` extracts an existing `*Error` (nil otherwise); `IsClientError` reports whether there is one |

## Options

| Option                | Default   | Description                                                                  |
|-----------------------|-----------|------------------------------------------------------------------------------|
| `WithInsecure`        | false     | Disables TLS and uses plaintext (dev/test only)                              |
| `WithTLSConfig`       | TLS 1.2+  | Custom `*tls.Config` for the connection                                     |
| `WithLogger`          | discard   | Structured logger for call logging and connection events                     |
| `WithMeter`           | nil       | Records outgoing unary and streaming RPC metrics                            |
| `WithTracer`          | nil       | Creates client spans and propagates W3C trace context                        |
| `WithAppName`         | --        | Application name sent in the gRPC User-Agent header                          |
| `WithRetry`           | disabled  | Enables retry (5 attempts, 1s-10s exponential backoff); `RetryOption` values override it |
| `WithoutHealthCheck`  | enabled   | Disables the client-side watch of overall server health |
| `WithPool`  | nil       | Accepts a `ConnPool`, including `*pool.ConnectionPool`                          |
| `WithInterceptors`    | --        | Appends named unary/stream client interceptors to the dependency-ordered chain |
| `WithDialOptions`     | --        | Appends custom `grpc.DialOption` values (auth credentials, call options)     |
| `WithProxy`           | env       | Route via `http://host:port` with optional `*url.Userinfo`                    |
| `WithProxyURL`        | env       | Route via any proxy URL (`http`, `https`, `socks5`, `socks5h`)               |
| `WithProxyFunc`       | env       | Custom resolver matching `http.Transport.Proxy` signature (escape hatch)     |
| `WithoutProxy`        | env       | Disable proxy resolution including `HTTPS_PROXY` env lookup                  |
| `WithProxyTLSConfig`  | system    | Custom `*tls.Config` for the handshake to an `https://` proxy (self-signed CA, mTLS, …) |
| `WithMutationTimeout` | 30s       | Default timeout for Create/Update/Delete operations                          |
| `WithQueryTimeout`    | 5s        | Default timeout for Get/List operations                                      |
| `WithErrorConverter`  | nil       | Custom error converter overriding the default `ParseStatusError`             |
| `WithHealthCoordinator`  | nil           | Opts the client into a `health.Reporter` (single-mode watcher / pool subscription) |
| `WithHealthServiceName`  | `grpc_client` | Service name registered with the coordinator                                          |
| `WithHealthStateMapper`  | default       | Override the `connectivity.State` → `health.ServingStatus` mapping                    |

## Observability

`WithMeter` enables the default [client RPC metrics](../interceptors/metrics) under the `grpc` subsystem.
`WithTracer` injects the client span's W3C trace context into outgoing metadata. The built-in chain runs metadata, error conversion, tracing, logging and metrics
in that order. On completion the observers see the wire status before conversion to a domain error. Call timing covers native retry attempts
and, for streams, ends when gRPC reports the final status. gRPC runs interceptors set with `grpc.WithUnaryInterceptor` or
`grpc.WithStreamInterceptor` before the chain.

## Client-side health checking

The client uses round-robin balancing and calls `grpc.health.v1.Health/Watch` with the empty service name to watch overall server health.
A subchannel reporting `NOT_SERVING` stops receiving calls until it reports `SERVING` again. With no serving subchannel, a fail-fast RPC
returns `Unavailable`. Pass `WithoutHealthCheck()` to disable the watch. `WithHealthCoordinator` reports the local connection state separately.

## Retry options

`WithRetry(opts ...RetryOption)` takes generated setters; unset settings keep their defaults.

| Option                       | Default                                               | Description                                  |
|------------------------------|-------------------------------------------------------|----------------------------------------------|
| `WithRetryMaxAttempts`       | 5 (`DefaultMaxRetryAttempts`)                         | Maximum attempts, including the initial call |
| `WithRetryInitialBackoff`    | 1s (`DefaultInitialBackoff`)                          | Initial backoff between retries              |
| `WithRetryMaxBackoff`        | 10s (`DefaultMaxBackoff`)                             | Maximum backoff                              |
| `WithRetryBackoffMultiplier` | 2.0 (`DefaultBackoffMultiplier`)                      | Exponential backoff multiplier               |
| `WithRetryStatusCodes`       | `DefaultRetryableStatusCodes`                         | gRPC status code names that trigger a retry  |

## Health

Pass `WithHealthCoordinator(coord)` to register the client with `observability/health`. The aggregate checker (default service name `grpc_client`)
reflects the [`grpc.ClientConn`](https://pkg.go.dev/google.golang.org/grpc#ClientConn) connectivity state through `pool.DefaultStateMapper`:

| `connectivity.State` | `ServingStatus` |
|----------------------|-----------------|
| `Ready` / `Idle` / `Connecting` | `Serving` |
| `TransientFailure`   | `Degraded`     |
| `Shutdown`           | `NotServing`   |

In **single-connection mode** the client owns a watcher goroutine that calls `grpc.ClientConn.WaitForStateChange` and pushes a status to the coordinator
on every transition.

In **pool mode** the client subscribes to the pool's state tracker via `pool.ConnectionPool.SubscribeTarget`. To monitor the pool itself across multiple
targets and clients, configure `pool.WithHealthCoordinator` on the pool — the two integrations are independent.

```go
coord := health.New()
defer coord.Close()

c, err := client.New(ctx, "service.example.com:443",
    client.WithHealthCoordinator(coord),
)
```

With `health.Coordinator`, closing a client releases its health registration.
If another client has registered the same service name, its checker and status remain active.
Use `WithScopedHealthReporter` for decorators that implement owner-scoped registration. `WithHealthCoordinator` calls the decorator's named methods.
A registration-only reporter receives a detachable checker that releases the client when closed. A coordinator receiving that checker also removes it.

## Subpackages

| Package                    | Description                                                          |
|----------------------------|----------------------------------------------------------------------|
| [pool](./pool)             | gRPC client connection pooling with automatic cleanup and monitoring |

## Pool policy

`WithPool(p)` applies the same TLS, proxy, retry, interceptor and custom dial options as single-connection mode. Each client binds its own connection
factory to the pool; connections are reused only within that binding. Bindings for the same address share the pool's capacity limit. Client policy takes
precedence over the pool's default factory. Return borrowed connections with `ReturnConnection`; the pool's stop function owns their shutdown.
