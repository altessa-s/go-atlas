# client

```go
import "github.com/altessa-s/go-atlas/transport/grpc/client"
```

Package `client` provides a generic gRPC client with connection management, retry, health checking, and error handling. It is designed as a
foundation for building service-specific gRPC clients with consistent behavior for connection pooling, automatic retry with exponential backoff,
structured logging, and rich error parsing with field-level validation details.

## Key types

| Type / Interface  | Description                                                                              |
|-------------------|------------------------------------------------------------------------------------------|
| `Client`          | Core gRPC client: single connection or pool mode, retry, timeouts, health checking       |
| `Error`           | Structured gRPC error with code, request ID, reason, metadata, and field validations     |
| `FieldError`      | Single field validation violation extracted from `BadRequest` error details               |
| `RetryConfig`     | Retry policy: max attempts, initial/max backoff, multiplier, retryable status codes      |
| `ErrorConverter`  | Hook to override default `ParseStatusError` behavior in the error-status interceptor     |

## Options

| Option                | Default   | Description                                                                  |
|-----------------------|-----------|------------------------------------------------------------------------------|
| `WithInsecure`        | false     | Disables TLS and uses plaintext (dev/test only)                              |
| `WithTLSConfig`       | TLS 1.2+  | Custom `*tls.Config` for the connection                                     |
| `WithLogger`          | discard   | Structured logger for call logging and connection events                     |
| `WithAppName`         | --        | Application name sent in the gRPC User-Agent header                          |
| `WithRetry`           | disabled  | Enables retry with default config (5 attempts, 1s-10s exponential backoff)   |
| `WithRetryConfig`     | --        | Enables retry with a custom `RetryConfig`                                    |
| `WithConnectionPool`  | nil       | Enables pool mode backed by a `pool.ConnectionPool`                          |
| `WithDialOptions`     | --        | Appends custom `grpc.DialOption` values (auth credentials, call options)     |
| `WithProxy`           | env       | Route via `http://host:port` with optional `*url.Userinfo`                    |
| `WithProxyURL`        | env       | Route via any proxy URL (`http`, `https`, `socks5`, `socks5h`)               |
| `WithProxyFunc`       | env       | Custom resolver matching `http.Transport.Proxy` signature (escape hatch)     |
| `WithoutProxy`        | env       | Disable proxy resolution including `HTTPS_PROXY` env lookup                  |
| `WithProxyTLSConfig`  | system    | Custom `*tls.Config` for the handshake to an `https://` proxy (self-signed CA, mTLS, …) |
| `WithMutationTimeout` | 30s       | Default timeout for Create/Update/Delete operations                          |
| `WithQueryTimeout`    | 5s        | Default timeout for Get/List operations                                      |
| `WithErrorConverter`  | nil       | Custom error converter overriding the default `ParseStatusError`             |

## Subpackages

| Package                    | Description                                                          |
|----------------------------|----------------------------------------------------------------------|
| [pool](./pool)             | gRPC client connection pooling with automatic cleanup and monitoring |
