# config

```go
import "github.com/altessa-s/go-atlas/config"
```

Package `config` defines configuration structures and validation logic for all infrastructure components managed by
go-atlas. Each struct carries YAML tags and default-value tags; the `config/loader` package populates them from files,
environment variables, and secret references.

## Features

- Typed structs for every infrastructure concern (MongoDB, NATS, Redis, gRPC, HTTP, TLS, Auth, etc.)
- `Default*` constructors with sensible defaults for each component
- `Validate` methods powered by [ozzo-validation](https://github.com/go-ozzo/ozzo-validation)
- `Normalize` methods that allocate provider sub-configs based on type-selector fields
- `Secret` type that auto-redacts credentials in fmt, JSON, YAML, and slog output
- Shared storage abstraction (`memory` / `redis` / `nats`) for rate limiting, idempotency, caching

## Structure types

Every top-level config struct exposes a consistent API: `Default*` constructors, `Normalize`, and `Validate`.
Call `Normalize()` before `Validate()` so the correct sub-struct is present.

## Storage pattern

Features that need distributed storage share `CacheStorageConfig`, which contains a `Type` selector and optional
sub-configs for `Memory`, `Nats`, and `Redis`.

## Interceptors and middlewares

gRPC interceptor configs embed `BaseGrpcInterceptorConfig` (enable/disable + method exclusion). HTTP middleware
configs embed `BaseHttpMiddlewareConfig` with path-based filtering. Concrete configs only declare their unique
fields.

## Secret handling

`Secret` wraps sensitive strings and redacts them in `fmt`, JSON, YAML, and `slog` output. Use the `Expose()`
method to retrieve the underlying value when needed.

## Helpers

| Function                    | Description                                          |
|-----------------------------|------------------------------------------------------|
| `ValidationErrorsToFlatMap` | Flatten nested `validation.Errors` to dot-notation   |
| `PrettyError`               | Human-readable multi-line validation error output    |
| `ValidateStruct`            | Nil-safe wrapper around `validation.ValidateStruct`  |
| `ValidateIgnoreConfig`      | Shared rules for method/pattern exclusion fields     |

## Subpackages

| Package                            | Description                                                  |
|------------------------------------|--------------------------------------------------------------|
| `config/loader`                    | Multi-source configuration loading                           |
| `config/loader/backend`            | Backend interface for file format parsers                    |
| `config/loader/backend/toml`       | TOML backend (`github.com/BurntSushi/toml`)                  |
| `config/loader/backend/yaml3`      | YAML backend (`gopkg.in/yaml.v3`) with `!include`            |
| `config/loader/secrets`            | `$__secret{ns:key}` placeholder expansion                    |
| `config/templates`                 | Pre-built config structs for common transport components     |
| `config/templates/grpc_interceptors` | Ready-to-use gRPC interceptor config structs               |
| `config/templates/http_middlewares` | Ready-to-use HTTP middleware config structs                 |
| `config/internal/utils`            | Internal file-lookup helpers                                 |
| `config/internal/validators`       | Internal custom validators (MongoDB)                         |
