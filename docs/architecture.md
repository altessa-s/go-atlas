# Architecture

Package structure, layering, and design principles of go-atlas.

---

## Package structure

```
go-atlas/
├── auth/                  # Authentication & authorization
│   ├── oidc/              # OpenID Connect (JWT validation, JWKS)
│   └── opa/               # Open Policy Agent integration
├── config/                # Configuration management
│   ├── loader/            # Multi-source config loading (YAML, TOML, env)
│   └── templates/         # Configuration templates (30+ YAML presets)
├── core/                  # Foundational utilities (stdlib only, zero external deps)
│   ├── collections/       # Generic collection utilities
│   ├── context/           # Context helpers
│   ├── encoding/          # Encoding utilities
│   ├── errors/            # Error types and wrapping
│   ├── factory/           # Generic factory pattern
│   ├── io/                # I/O utilities
│   ├── net/               # Network utilities
│   ├── runtime/           # Runtime helpers (concurrency strategies)
│   ├── scheduler/         # Task scheduling interfaces
│   ├── text/              # String and text utilities (interning)
│   ├── time/              # Time utilities
│   └── types/             # Common type definitions
├── data/                  # Data access & patterns
│   ├── audit/             # Audit logging
│   ├── cache/             # Multi-backend caching (Redis, FreeCache, LRU)
│   ├── filter/            # Query filtering (CEL expressions)
│   ├── idempotency/       # Idempotency key management
│   ├── leadelect/         # Leader election
│   ├── limiters/          # Rate limiting (token bucket, budget)
│   ├── locks/             # Distributed locking
│   ├── mongo/             # MongoDB repository patterns
│   ├── outbox/            # Transactional outbox
│   ├── probfilter/        # Probabilistic filters (Bloom, Cuckoo)
│   └── uniq/              # Uniqueness constraints
├── domain/                # Domain logic helpers
│   ├── behavior/          # field_behavior strip (create/update/response)
│   ├── converter/         # Struct-to-struct conversion
│   ├── eventbus/          # Synchronous in-process event bus (transaction-safe)
│   ├── fieldtracker/      # Field change tracking
│   ├── normalizer/        # Data normalization
│   ├── proto/             # Protobuf utilities (field masks)
│   └── validation/        # Validation utilities (ISO 7064)
├── infrastructure/        # External system connectors
│   ├── mongo/             # MongoDB client setup
│   ├── nats/              # NATS connection management
│   └── redis/             # Redis client setup
├── observability/         # Observability stack
│   ├── appstats/          # Application statistics
│   ├── health/            # Health checks
│   ├── metrics/           # Metrics collection (Prometheus)
│   ├── slog/              # Structured logging (slog)
│   └── tracing/           # Distributed tracing (OpenTelemetry)
├── proto/                 # Protobuf definitions and generated code
├── security/              # Security utilities
│   ├── secrets/           # Secret management (Vault, GCP, Lockbox, memory)
│   ├── tlsutils/          # TLS certificate helpers
│   └── vault/             # HashiCorp Vault integration
├── service/               # Service-level components
│   ├── id/                # ID generation (UUID, ULID)
│   └── scheduler/         # Task scheduler (cron, priority, storage)
├── tools/                 # Code generation and dev tools
│   └── codegen/           # Code generators (optgen, goconfig)
└── transport/             # Communication layer
    ├── broker/            # Message broker (NATS JetStream)
    ├── grpc/              # gRPC server, interceptors, factory
    │   └── client/        # gRPC client with retry, pooling, proxy support
    ├── http/              # HTTP server, router, middleware, codec
    │   └── client/        # HTTP client with retry, breaker, proxy, SSRF protection
    └── internal/
        └── proxydial/     # Shared HTTP CONNECT/SOCKS5 dialer for both clients
```

---

## Layering

```
┌──────────────────────────────────────────────────────────┐
│                      Application                         │
├──────────────────────────────────────────────────────────┤
│  transport/       │  auth/       │  service/             │
│  (HTTP, gRPC,     │  (OIDC,      │  (ID gen,             │
│   broker)         │   OPA)       │   scheduling)         │
├──────────────────────────────────────────────────────────┤
│  data/            │  config/     │  observability/       │
│  (cache, mongo,   │  (loader,    │  (metrics, tracing,   │
│   outbox, ...)    │   templates) │   logging, health)    │
├──────────────────────────────────────────────────────────┤
│  domain/          │  security/   │  infrastructure/      │
│  (converter,      │  (secrets,   │  (mongo, redis,       │
│   normalizer)     │   vault)     │   nats clients)       │
├──────────────────────────────────────────────────────────┤
│                        core/                             │
│  (collections, errors, types, context, encoding, ...)    │
└──────────────────────────────────────────────────────────┘
```

Higher layers depend on lower layers. Lateral dependencies within the same layer are allowed. Circular dependencies between top-level packages are
prohibited.

### Outbound transport

The HTTP and gRPC client packages (`transport/http/client`, `transport/grpc/client`) share a common dialer at `transport/proxydial`. Every consumer
that makes outbound calls (OIDC, OPA GitLab/S3 sources, OTLP gRPC exporter) materializes `config.HTTPProxy` / `config.GrpcProxy` into option slices via
`ClientOptions()` and forwards them to the relevant client. See the [Proxy guide](proxy.md) for the YAML schema, modes, and wiring patterns.

---

## Design principles

### Interface-driven

Every major component is defined by an interface. Implementations are injected, making components testable and swappable. Storage backends, secret
providers, cache providers, and observability adapters all follow this pattern.

### Factory pattern

Components support both programmatic construction (`New()` + functional options) and configuration-driven creation (`factory.New(cfg).Build()`), so
the same package works as a library or an app-level component. Factory subdirectories appear in 20+ packages and follow a consistent fluent builder
API with deferred error accumulation.

### Optional dependencies

External dependencies (tracing, metrics, logging) are accepted through functional options and default to no-op implementations. Packages work without
configuration.

### Adapter pattern

Observability (tracing, metrics), infrastructure (secrets, cache providers), and data access (filter translators, storage backends) use the adapter
pattern: components depend on abstract interfaces, adapters translate to specific backends.

| Domain          | Adapters                                                     |
|-----------------|--------------------------------------------------------------|
| Cache           | Redis, FreeCache, LRU, Noop                                  |
| Filter          | Lua, MongoDB (BSON), RediSearch                              |
| Idempotency     | Memory, NATS, Redis                                          |
| Leader election | NATS                                                         |
| Rate limiting   | Memory, NATS, Redis                                          |
| Metrics         | Prometheus                                                   |
| Tracing         | OpenTelemetry                                                |
| Secrets         | Vault, GCP Secret Manager, Yandex Cloud Lockbox, Memory      |
| OPA sources     | Embed, Filesystem, GitLab, S3                                |

### No global state

All state is held in structs. No `init()` functions, no package-level variables holding mutable state. Concurrent usage is safe and testing is
deterministic.

### Minimal public API

Only export what users need. Internal packages (`internal/`) hide implementation details. Generated code (`*_gen.go`, `*.pb.go`) is clearly separated.

---

## Dependency rules

| Package           | May depend on                                                                  |
|-------------------|--------------------------------------------------------------------------------|
| `core/`           | Standard library only (zero external deps)                                     |
| `domain/`         | `core/`, `proto/`, `observability/`, `data/cache/lru` (field mask caching)     |
| `data/`           | `core/`, external libraries                                                    |
| `infrastructure/` | `core/`, `config/`, external client libraries                                  |
| `observability/`  | `core/`, adapter libraries (Prometheus, OpenTelemetry)                         |
| `transport/`      | `core/`, `observability/`, `config/`, `data/`, `security/`, protocol libraries |

Circular dependencies between top-level packages are not allowed.
