# go-atlas

Production-ready Go building blocks for distributed systems — transport, caching, observability, auth, secrets, and more.

[![Go Reference](https://pkg.go.dev/badge/github.com/altessa-s/go-atlas.svg)](https://pkg.go.dev/github.com/altessa-s/go-atlas)
[![CI](https://github.com/altessa-s/go-atlas/actions/workflows/ci.yml/badge.svg)](https://github.com/altessa-s/go-atlas/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/altessa-s/go-atlas)](https://goreportcard.com/report/github.com/altessa-s/go-atlas)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/github/v/release/altessa-s/go-atlas)](https://github.com/altessa-s/go-atlas/releases)


## Overview

go-atlas is a modular Go toolkit for building production-grade distributed services. It provides a composable set of packages — each usable independently — covering transport (HTTP, gRPC, NATS), caching, observability, authentication, secret management, configuration, and domain utilities.

Every major component is defined by an interface, implementations are injected via functional options, and unset dependencies default to safe no-ops. Import only what you need — there is no framework bootstrap or global state.

## Requirements

- **Go 1.25+**

## Installation

```bash
go get github.com/altessa-s/go-atlas
```

Import only the packages you need:

```go
import "github.com/altessa-s/go-atlas/data/cache"
```

## Package Overview

| Package | Description |
|---------|-------------|
| [`auth/oidc`](auth/oidc/) | OIDC/JWT validation with JWKS auto-refresh, CEL rules, token revocation |
| [`auth/opa`](auth/opa/) | Open Policy Agent with bundle hot-reloading |
| [`config/loader`](config/loader/) | Multi-source config loading (YAML/TOML, env vars, secrets) with validation |
| [`config/templates`](config/templates/) | Pre-built configuration structs for common components |
| [`core`](core/) | Collections, errors, context, encoding, retry, scheduling, types (zero external deps) |
| [`data/audit`](data/audit/) | Async audit-event dispatcher with pluggable storage |
| [`data/cache`](data/cache/) | Multi-backend caching with singleflight, fallback, and TTL management |
| [`data/idempotency`](data/idempotency/) | Idempotency key management (Redis, NATS, memory) |
| [`data/leadelect`](data/leadelect/) | Leader election (NATS KV-based) |
| [`data/limiters`](data/limiters/) | Token-bucket rate limiting (Redis, NATS, memory) |
| [`data/locks`](data/locks/) | Distributed locking (NATS) |
| [`data/mongo`](data/mongo/) | MongoDB repository patterns, cursor pagination, CSFLE, migrations |
| [`data/outbox`](data/outbox/) | Transactional outbox (MongoDB-backed) |
| [`data/probfilter`](data/probfilter/) | Bloom and Cuckoo probabilistic filters |
| [`data/uniq`](data/uniq/) | Uniqueness constraint providers (NATS, Redis, no-op) |
| [`domain/converter`](domain/converter/) | Generic struct-to-struct conversion with codecs and lazy iterators |
| [`domain/fieldtracker`](domain/fieldtracker/) | Struct field change tracking |
| [`domain/normalizer`](domain/normalizer/) | Tag-driven data normalization with pluggable modifiers |
| [`domain/proto`](domain/proto/) | Protobuf field mask utilities |
| [`infrastructure/mongo`](infrastructure/mongo/) | MongoDB client setup and lifecycle |
| [`infrastructure/nats`](infrastructure/nats/) | NATS connection management |
| [`infrastructure/redis`](infrastructure/redis/) | Redis client setup |
| [`observability/health`](observability/health/) | Health check coordinator with subscriptions |
| [`observability/metrics`](observability/metrics/) | Prometheus metrics via interface-driven adapters |
| [`observability/slog`](observability/slog/) | slog extensions: nil-safe helpers, colorized and PII-masking handlers |
| [`observability/tracing`](observability/tracing/) | Distributed tracing (OpenTelemetry, OTLP, console) with samplers |
| [`security/secrets`](security/secrets/) | Generic secret manager with LRU cache, watch, and scheduler refresh |
| [`security/tlsutils`](security/tlsutils/) | TLS helpers, OCSP stapling, Let's Encrypt (Certify), Vault-backed certs |
| [`security/vault`](security/vault/) | HashiCorp Vault client (AppRole, token, userpass auth) |
| [`service/id`](service/id/) | Service identity (ULID/UUID) from env, file, or static |
| [`service/scheduler`](service/scheduler/) | Distributed cron scheduler with priority queues and leader election |
| [`tools/codegen/optgen`](tools/codegen/optgen/) | Code generator for functional options pattern |
| [`tools/codegen/goconfig`](tools/codegen/goconfig/) | Configuration struct code generator |
| [`transport/broker`](transport/broker/) | Message broker abstraction (NATS JetStream, outbox) |
| [`transport/grpc`](transport/grpc/) | gRPC server, interceptors, factory |
| [`transport/http`](transport/http/) | HTTP server, middlewares, codec registry, HTTP client |

## Documentation

Full API documentation is available at [pkg.go.dev](https://pkg.go.dev/github.com/altessa-s/go-atlas).

## Development

```bash
git clone https://github.com/altessa-s/go-atlas.git
cd go-atlas
```

| Command | Description |
|---------|-------------|
| `make fmt` | Format code and sort imports |
| `make lint` | Run golangci-lint (34 linters) |
| `make test` | Run all tests |
| `make test-all` | Tests with race detector, shuffle, and coverage |
| `make bench` | Run all benchmarks |
| `make security-scan` | Run `govulncheck` and `gosec` |
| `make generate` | Run `go generate` across all packages |

## Contributing

Contributions are welcome. Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Security

Report vulnerabilities by email to **security@altessa-s.com**. See [SECURITY.md](SECURITY.md).

## License

MIT License — Copyright (c) 2021-2026 [ALTESSA SOLUTIONS INC](https://altessa-s.com). See [LICENSE](LICENSE).
