# auth

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
```

Package `auth` provides gRPC interceptors for token-based authentication and scope-based authorization. ServerInterceptor validates tokens via a
pluggable `Auth` function, ClientInterceptor injects tokens into outgoing metadata. ScopeRegistry maps gRPC methods to required OAuth scopes.

## Key types

| Type / Interface  | Description                                                                  |
|-------------------|------------------------------------------------------------------------------|
| `Auth`            | Interface for validating authentication credentials                          |
| `AuthFunc`        | Function adapter for `Auth`                                                  |
| `ClientAuth`      | Interface for additional client-level authentication after initial auth       |
| `TokenExtractor`  | Interface for extracting tokens from context (default: Bearer from metadata) |
| `TokenProvider`   | Interface for providing tokens in outgoing client requests                    |
| `ScopeRegistry`   | Maps gRPC method names to required authorization scopes (O(1) lookup)        |
| `Credentials`     | Authenticated credentials with headers and custom data                       |
| `Request`         | Authentication request with method, token, and metadata                      |

## Server options

| Option               | Default            | Description                                      |
|----------------------|--------------------|--------------------------------------------------|
| `WithAuthFn`         | --                 | Authentication function (required)                |
| `WithClientAuth`     | nil                | Additional client-level auth check                |
| `WithTokenExtractor` | Bearer token       | Custom token extraction logic                     |
| `WithIgnoreMethods`  | --                 | Methods to skip authentication                    |
| `WithIgnorePatterns` | reflection, health | Regex patterns for methods to skip                |
| `WithLogger`         | discard            | Structured logger                                 |

## Client options

| Option           | Default         | Description                                       |
|------------------|-----------------|---------------------------------------------------|
| `WithHeaderName` | "authorization" | Metadata header name for the token                |
| `WithScheme`     | "Bearer"        | Authentication scheme prefix (empty for none)     |

## Subpackages

| Package              | Description                                      |
|----------------------|--------------------------------------------------|
| [oidc](./oidc)       | OIDC token validation and claims extraction       |
