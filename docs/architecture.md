[Back to README](../README.md) · [Configuration →](configuration.md)

# Architecture

This document describes the package structure, layering, and design principles of go-atlas.

## Package Structure

```
go-atlas/
├── auth/                  # Authentication & authorization
│   ├── oidc/              # OpenID Connect (JWT validation, JWKS)
│   └── opa/               # Open Policy Agent integration
├── config/                # Configuration management
│   ├── loader/            # Multi-source config loading (YAML, TOML, env)
│   └── templates/         # Configuration templates
├── core/                  # Foundational utilities (no external deps)
│   ├── collections/       # Generic collection utilities
│   ├── context/           # Context helpers
│   ├── encoding/          # Encoding utilities
│   ├── errors/            # Error types and wrapping
│   ├── factory/           # Generic factory pattern
│   ├── io/                # I/O utilities
│   ├── net/               # Network utilities
│   ├── runtime/           # Runtime helpers
│   ├── scheduler/         # Task scheduling
│   ├── text/              # String and text utilities
│   ├── time/              # Time utilities
│   └── types/             # Common type definitions
├── data/                  # Data access & patterns
│   ├── audit/             # Audit logging
│   ├── cache/             # Multi-backend caching (Redis, FreeCache, LRU)
│   ├── filter/            # Query filtering
│   ├── idempotency/       # Idempotency key management
│   ├── leadelect/         # Leader election
│   ├── limiters/          # Rate limiting
│   ├── locks/             # Distributed locking
│   ├── mongo/             # MongoDB repository patterns
│   ├── outbox/            # Transactional outbox
│   ├── probfilter/        # Probabilistic filters (Bloom, Cuckoo)
│   └── uniq/              # Uniqueness constraints
├── domain/                # Domain logic helpers
│   ├── converter/         # Struct-to-struct conversion
│   ├── fieldtracker/      # Field change tracking
│   ├── normalizer/        # Data normalization
│   └── proto/             # Protobuf utilities (field masks)
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
│   ├── secrets/           # Secret management abstraction
│   ├── tlsutils/          # TLS certificate helpers
│   └── vault/             # HashiCorp Vault integration
├── service/               # Service-level utilities
│   ├── id/                # ID generation (UUID, ULID)
│   └── scheduler/         # Service-level scheduling
├── tools/                 # Code generation and dev tools
│   └── codegen/           # Code generators (optgen, goconfig)
└── transport/             # Communication layer
    ├── broker/            # Message broker (NATS JetStream)
    ├── grpc/              # gRPC server, interceptors, factory
    ├── http/              # HTTP server, router, middleware, codec
    └── internal/          # Shared transport internals
```

## Layer Diagram

```
┌─────────────────────────────────────────────────────────┐
│                      Application                         │
├─────────────────────────────────────────────────────────┤
│  transport/    │  auth/     │  service/                  │
│  (HTTP, gRPC,  │  (OIDC,    │  (ID gen,                 │
│   broker)      │   OPA)     │   scheduling)              │
├─────────────────────────────────────────────────────────┤
│  data/         │  config/   │  observability/            │
│  (cache, mongo,│  (loader,  │  (metrics, tracing,        │
│   outbox, ...)│   templates)│   logging, health)         │
├─────────────────────────────────────────────────────────┤
│  domain/       │  security/ │  infrastructure/           │
│  (converter,   │  (secrets, │  (mongo, redis,            │
│   normalizer)  │   vault)   │   nats clients)            │
├─────────────────────────────────────────────────────────┤
│                       core/                              │
│  (collections, errors, types, context, encoding, ...)    │
└─────────────────────────────────────────────────────────┘
```

## Design Principles

### 1. Interface-Driven

Every major component is defined by an interface. Implementations are injected, making
components testable and swappable.

### 2. Factory Pattern

Components support both programmatic construction (via `New()` + functional options) and
configuration-driven creation (via `factory.CreateFromConfig()`). This enables both
library and application usage.

### 3. Optional Dependencies

External dependencies (tracing, metrics, logging) are accepted through functional options
and default to no-op implementations. Packages work out of the box with zero configuration.

### 4. Adapter Pattern

Observability (tracing, metrics) and infrastructure (secrets, cache providers) use the
adapter pattern: components depend on abstract interfaces, and adapters translate to specific
backends (OpenTelemetry, Prometheus, Vault, Redis, etc.).

### 5. No Global State

All state is held in structs. No `init()` functions, no package-level variables that hold
mutable state. This makes concurrent usage safe and testing deterministic.

### 6. Minimal Public API

Only export what users need. Internal packages (`internal/`) hide implementation details.
Generated code (`*_gen.go`, `*.pb.go`) is clearly separated.

## Dependency Rules

- `core/` has zero external dependencies (only stdlib).
- `domain/` depends only on `core/` and `proto/`.
- `data/` and `infrastructure/` may depend on external libraries.
- `transport/` depends on `core/`, `observability/`, and protocol libraries.
- Circular dependencies between top-level packages are not allowed.
