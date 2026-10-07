# config

```go
import "github.com/altessa-s/go-atlas/config/<capability>"
```

The `config` tree holds the configuration schemas of go-atlas, one package per capability. Each struct carries YAML tags and default-value
tags; [`config/loader`](loader) populates them from files, environment variables and secret references. The root package declares no types.

## Rules

- Schemas describe settings only. A schema package imports `core/*`, other schema packages and `config/internal`, never runtime
  components.
- Runtime packages never import schemas; they take generated options. Component factories (`*/factory`) read schemas and map them to
  options. `make check-architecture` enforces both directions.
- Package names carry a `config` suffix (`grpcconfig`, `redisconfig`) so they do not clash with the driver packages factories import.
- Every top-level schema exposes `Default*` constructors where defaults exist, `Normalize` where a type selector allocates a
  sub-config, and `Validate`. Call `Normalize()` before `Validate()`.
- Secret fields use [`redacted.RedactedString`](../core/types/redacted), which redacts the value in `fmt`, JSON, YAML and `slog` output;
  `Expose()` returns it.

## Schema packages

| Package                                 | Name                  | Schemas                                                                                                                         |
|-----------------------------------------|-----------------------|---------------------------------------------------------------------------------------------------------------------------------|
| [`config/validation`](validation)       | `validationconfig`    | Shared validation helpers for configuration schemas: struct validation with readable errors and storage-selector checks         |
| [`config/storage`](storage)             | `storageconfig`       | Shared storage-backend schemas: cache storage selection (memory, NATS, Redis) and NATS JetStream KeyValue settings              |
| [`config/middleware`](middleware)       | `middlewareconfig`    | Schema pieces shared by gRPC interceptors and HTTP middlewares: enable toggles, fallback behavior and IP/geo ACL rules          |
| [`config/s3`](s3)                       | `s3config`            | The S3 connection schema shared by TLS certificate providers and OPA bundle sources                                             |
| [`config/proxy`](proxy)                 | `proxyconfig`         | The outbound HTTP proxy schema used by OIDC, OPA and tracing exporters                                                          |
| [`config/retry`](retry)                 | `retryconfig`         | The retry policy schema used by outbound clients                                                                                |
| [`config/tls`](tls)                     | `tlsconfig`           | TLS client and server schemas and the certificate provider schemas (file, Let's Encrypt, OCSP, S3, Vault)                       |
| [`config/clienthealth`](clienthealth)   | `clienthealthconfig`  | The client health-check schema for outbound gRPC and HTTP clients                                                               |
| [`config/redis`](redis)                 | `redisconfig`         | The Redis connection schema                                                                                                     |
| [`config/nats`](nats)                   | `natsconfig`          | The NATS connection, JetStream consumer and recovery schemas                                                                    |
| [`config/mongo`](mongo)                 | `mongoconfig`         | The MongoDB connection, credential and client-side field level encryption schemas                                               |
| [`config/clickhouse`](clickhouse)       | `clickhouseconfig`    | The ClickHouse connection schema                                                                                                |
| [`config/meilisearch`](meilisearch)     | `meilisearchconfig`   | The Meilisearch connection schema                                                                                               |
| [`config/vault`](vault)                 | `vaultconfig`         | The HashiCorp Vault client schema                                                                                               |
| [`config/secrets`](secrets)             | `secretsconfig`       | The secret manager schema                                                                                                       |
| [`config/observability`](observability) | `observabilityconfig` | Logging, metrics, tracing and health schemas and the aggregate Observability block                                              |
| [`config/probfilter`](probfilter)       | `probfilterconfig`    | The probabilistic filter (Bloom, Cuckoo) schemas                                                                                |
| [`config/auth`](auth)                   | `authconfig`          | Authentication and authorization schemas: OIDC, OPA, mTLS, scopes, denylist, OAuth2 client, SPIFFE and the aggregate Auth block |
| [`config/grpc`](grpc)                   | `grpcconfig`          | The gRPC server schema and the schemas of its interceptors                                                                      |
| [`config/http`](http)                   | `httpconfig`          | The HTTP server schema, its middlewares, pprof, TLS and outbound SSRF protection                                                |
| [`config/broker`](broker)               | `brokerconfig`        | The message broker schema with its outbox and in-progress tracking blocks                                                       |
| [`config/idempotency`](idempotency)     | `idempotencyconfig`   | The idempotency key storage schema                                                                                              |
| [`config/limiter`](limiter)             | `limiterconfig`       | Rate limiter schemas: token bucket, request budget and request limiter                                                          |
| [`config/lock`](lock)                   | `lockconfig`          | Distributed lock and leader election schemas                                                                                    |
| [`config/saga`](saga)                   | `sagaconfig`          | The saga orchestration schema                                                                                                   |
| [`config/scheduler`](scheduler)         | `schedulerconfig`     | The distributed scheduler schema                                                                                                |
| [`config/dispatch`](dispatch)           | `dispatchconfig`      | The async dispatch engine schema and its write-ahead log                                                                        |
| [`config/audit`](audit)                 | `auditconfig`         | The audit event dispatcher schema                                                                                               |
| [`config/plugins`](plugins)             | `pluginsconfig`       | The plugin manager and plugin sandbox schemas                                                                                   |
| [`config/webhook`](webhook)             | `webhookconfig`       | The webhook signing schema                                                                                                      |
| [`config/node`](node)                   | `nodeconfig`          | The service node identity schema                                                                                                |

## Shared patterns

Features that need distributed storage share `storageconfig.CacheStorageConfig`: a `Type` selector with optional `Memory`, `Nats` and
`Redis` sub-configs. gRPC interceptor schemas embed `grpcconfig.BaseInterceptor` and HTTP middleware schemas embed
`httpconfig.BaseMiddleware`; both build on `middlewareconfig.EnableMixin`, `middlewareconfig.FallbackBehavior` and the shared ACL
rule types. `validationconfig` provides `ValidateStruct`, `PrettyError` and `ValidationErrorsToFlatMap`.

## Other subpackages

| Package                       | Description                                         |
|-------------------------------|-----------------------------------------------------|
| `config/loader`               | Multi-source configuration loading                  |
| `config/loader/backend`       | Backend interface for file format parsers           |
| `config/loader/backend/toml`  | TOML backend (`github.com/BurntSushi/toml`)         |
| `config/loader/backend/yaml3` | YAML backend (`gopkg.in/yaml.v3`) with `!include`   |
| `config/loader/secrets`       | `$__secret{ns:key}` placeholder expansion          |
| `config/templates`            | Commented YAML templates for every schema           |
| `config/internal/utils`       | Internal file-lookup helpers                        |
| `config/internal/validators`  | Internal custom validators (MongoDB)                |
