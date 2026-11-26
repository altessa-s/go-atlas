# go-atlas

Production-ready Go building blocks for distributed systems — transport, caching, observability, auth, secrets, and more.

[![Go Reference](https://pkg.go.dev/badge/github.com/altessa-s/go-atlas.svg)](https://pkg.go.dev/github.com/altessa-s/go-atlas)
[![CI](https://github.com/altessa-s/go-atlas/actions/workflows/ci.yml/badge.svg)](https://github.com/altessa-s/go-atlas/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/altessa-s/go-atlas)](https://goreportcard.com/report/github.com/altessa-s/go-atlas)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/github/v/release/altessa-s/go-atlas)](https://github.com/altessa-s/go-atlas/releases)


## Overview

go-atlas is a modular Go toolkit for distributed services. Each package is usable on its own and covers one concern: transport (HTTP, gRPC, NATS),
caching, observability, authentication, secret management, configuration, and domain utilities.

Every major component is defined by an interface, implementations are injected via functional options, and unset dependencies default to safe no-ops.
Import the packages you need; applications own construction and lifecycle. See the [architecture](docs/architecture.md) for dependency and state
ownership contracts.

> **History.** go-atlas started in 2022 as an internal company project and ran in production for several years before it was extracted and open-sourced
> on GitHub. As a result, the public git history begins at the point of import rather than at the project's actual inception.


> **On AI tooling.** Yes, we use AI assistants on this project — deliberately and with purpose. Every change is reviewed, tested, and held to the same
> standards as hand-written code. AI accelerates well-understood work; it does not replace design decisions or engineering judgment.

## Requirements

- **Go 1.27+**

## Installation

```bash
go get github.com/altessa-s/go-atlas
```

Import only the packages you need:

```go
import "github.com/altessa-s/go-atlas/cache"
```

The root `go.mod` declares every integration go-atlas supports (OPA, AWS, GCP, Vault, NATS, Redis, and more), so all of them appear in a consumer's
`go.sum`. Go's build graph is per-package: only the packages you actually import are compiled and linked into your binary. Heavy integrations live
behind their own subpackages, so an unused subsystem costs you `go.sum` entries — never build time or binary size.

The package overview includes capabilities from go-atlas and its companion modules, such as `go-atlas-core` and `go-atlas-net`. Those companion
packages are maintained in separate repositories; [the architecture guide](docs/architecture.md) distinguishes them from packages in this module.

The AIP request contracts — filtering, ordering, projection, listing, page tokens, field masks and field behavior — live in
[go-aip](https://github.com/altessa-s/go-aip). go-atlas wires them into the framework: the page-token factory
([`factory/ext/aip/pagination/pagetoken`](factory/ext/aip/pagination/pagetoken/)), the gRPC [`fieldmask`](transport/grpc/interceptors/fieldmask/) and
[`fieldbehavior`](transport/grpc/interceptors/fieldbehavior/) interceptors, the HTTP response sanitizer, keyset paging in
[`store/mongo/keyset`](store/mongo/keyset/) and the scheduler handler. Production code imports only the root go-aip module; an application adds
the go-aip storage backends it uses (`github.com/altessa-s/go-aip/backend/<name>`).

**MongoDB upgrade.** In keyset filters, replace arbitrary Go structs and custom BSON marshalers with `bson.D` or
pre-built `bson.Raw`. For a scalar struct codec, call `bsonfilter.AdmitScalar` on the registry that encodes it.
Older tokens issued with a different map key order return `pagetoken.ErrFilterChanged`. Restart those reads from the first page.
The decimal codec now rejects coefficients over 4096 bits. See the [keyset migration notes](store/mongo/keyset/README.md#migrating-keyset-filters).

MongoDB repository reads now issue independent queries by default. Pass `mongo.WithReadDeduplication()` to opt into
[sharing in-flight queries](store/mongo/README.md#read-deduplication-and-tenant-isolation).
MongoDB migrations can register [cleanup for a failed up step](store/mongo/migrations/README.md#cleanup-after-a-failed-step).

## Package Overview

| Package                                                                             | Description                                                                                                                                                         |
| ----------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [`audit/decision`](audit/decision/)                                                 | Authorization decision audit trail — durable, structured allow/deny events with pluggable sinks                                                                     |
| [`auth/denylist`](auth/denylist/)                                                   | Token revocation denylist (JWT ID or subject), permanent or TTL-bound, consulted by verifiers                                                                       |
| [`reliability/idempotency/principalowner`](reliability/idempotency/principalowner/) | Adapter that binds idempotency records to the verified principal in context                                                                                         |
| [`auth/jwt`](auth/jwt/)                                                             | Generic JWT signing and verification toolkit shared by the auth packages                                                                                            |
| [`auth/mtls`](auth/mtls/)                                                           | Verified mTLS client certificate to authenticated principal, with pluggable validators                                                                              |
| [`auth/oauth2client`](auth/oauth2client/)                                           | OAuth2 token acquisition: client credentials, refresh, auth code, RFC 8693 exchange                                                                                 |
| [`auth/oidc`](auth/oidc/)                                                           | OIDC/JWT validation with JWKS auto-refresh, CEL rules, token revocation                                                                                             |
| [`auth/opa`](auth/opa/)                                                             | Open Policy Agent with bundle hot-reloading                                                                                                                         |
| [`auth/principal`](auth/principal/)                                                 | Canonical verified-identity type: subject, tenant, scopes, roles, raw claims                                                                                        |
| [`auth/scope`](auth/scope/)                                                         | Deny-by-default scope authorization policy with gRPC and HTTP adapters                                                                                              |
| [`auth/selfjwt`](auth/selfjwt/)                                                     | Self-issued JWT minting and verification with per-subject keys and rotation                                                                                         |
| [`auth/spiffe`](auth/spiffe/)                                                       | SPIFFE ID parsing from X.509 certificates for workload identity                                                                                                     |
| [`auth/static`](auth/static/)                                                       | Static token / API-key validator for service-to-service auth                                                                                                        |
| [`config`](config/)                                                                 | Configuration structs and validation for all go-atlas components                                                                                                    |
| [`config/templates`](config/templates/)                                             | Commented YAML configuration templates for the config structs                                                                                                       |
| [`audit`](audit/)                                                                   | Async audit-event dispatcher with pluggable storage                                                                                                                 |
| [`cache`](cache/)                                                                   | Multi-backend caching with singleflight, fallback, and TTL management                                                                                               |
| [`reliability/idempotency`](reliability/idempotency/)                               | Idempotency key management (Redis, NATS, MongoDB, SQL, memory)                                                                                                      |
| [`reliability/requestidstore`](reliability/requestidstore/)                         | Distributed AIP-155 request ID stores for the go-aip Guard (Redis, MongoDB, SQL, NATS KV)                                                                           |
| [`coord/leadelect`](coord/leadelect/)                                               | Leader election (NATS KV-based)                                                                                                                                     |
| [`coord/limiters`](coord/limiters/)                                                 | Token-bucket rate limiting (Redis, memory)                                                                                                                          |
| [`coord/dlock`](coord/dlock/)                                                       | Distributed locking (NATS, MongoDB, SQL)                                                                                                                            |
| [`store/meilisearch`](store/meilisearch/)                                           | Meilisearch SDK wrapper with context propagation and sentinel-error classification                                                                                  |
| [`store/mongo`](store/mongo/)                                                       | MongoDB repository patterns, cursor pagination, CSFLE, migrations                                                                                                   |
| [`store/postgres`](store/postgres/)                                                 | PostgreSQL pools, transactions and query tracing                                                                                                                    |
| [`store/postgres/migrations`](store/postgres/migrations/)                           | Goose migrations over a direct PostgreSQL connection (separate Go module)                                                                                           |
| [`store/postgres/postgrestest`](store/postgres/postgrestest/)                       | Isolated PostgreSQL databases for integration tests                                                                                                                 |
| [`reliability/outbox`](reliability/outbox/)                                         | Transactional outbox (MongoDB, PostgreSQL, MySQL, MariaDB)                                                                                                          |
| [`cache/probfilter`](cache/probfilter/)                                             | Bloom and Cuckoo probabilistic filters                                                                                                                              |
| [`reliability/retryqueue`](reliability/retryqueue/)                                 | Durable retry queue: one record per key, due list, backoff, expiry, generation fencing (memory, MongoDB, SQL)                                                       |
| [`reliability/saga`](reliability/saga/)                                             | Orchestration-based saga engine: sequential steps, compensating rollbacks, crash recovery                                                                           |
| [`mapping/behavior`](mapping/behavior/)                                             | Struct-tag `field_behavior` strip for Create / Update / Response payloads                                                                                           |
| [`messaging/eventbus`](messaging/eventbus/)                                         | Synchronous, lock-free, transaction-safe in-process event bus for decoupling domains                                                                                |
| [`mapping/fieldtracker`](mapping/fieldtracker/)                                     | Struct field change tracking                                                                                                                                        |
| [`domain/money`](domain/money/)                                                     | Currency amounts with exact decimal arithmetic, minor-unit rounding, allocation without losing a minor unit, and exchange rates (normalize, invert, cross, convert) |
| [`domain/money/moneypb`](domain/money/moneypb/)                                     | `money.Money` ↔ `google.type.Money`                                                                                                                                 |
| [`factory`](factory/)                                                               | Composition layer: one config-driven builder package per capability (`factory/<path>` builds `<path>`)                                                              |
| [`factory/core`](factory/core/)                                                     | Base type for factory pattern implementations                                                                                                                       |
| [`factory/ext/aip/pagination/pagetoken`](factory/ext/aip/pagination/pagetoken/)     | Paging configuration mapped to a go-aip page-token codec: signing key, rotation, TTL and encryption                                                                 |
| [`factory/ext/horai`](factory/ext/horai/)                                           | Scheduler configuration mapped to a go-horai scheduler, metrics, and the Mongo, Redis and ClickHouse factories                                                      |
| [`factory/infrastructure/clickhouse`](factory/infrastructure/clickhouse/)           | ClickHouse connection setup and lifecycle                                                                                                                           |
| [`factory/infrastructure/meilisearch`](factory/infrastructure/meilisearch/)         | Meilisearch client setup and lifecycle                                                                                                                              |
| [`factory/infrastructure/mongo`](factory/infrastructure/mongo/)                     | MongoDB client setup and lifecycle                                                                                                                                  |
| [`factory/infrastructure/nats`](factory/infrastructure/nats/)                       | NATS connection management                                                                                                                                          |
| [`factory/infrastructure/redis`](factory/infrastructure/redis/)                     | Redis client setup                                                                                                                                                  |
| [`factory/store/postgres`](factory/store/postgres/)                                 | PostgreSQL pool group setup, TLS wiring, startup migration hook, and health registration                                                                            |
| [`factory/ext/kryptos`](factory/ext/kryptos/)                                       | Secrets configuration mapped to a go-kryptos manager, scheduler refresh, and the Vault, GCP and Lockbox factories                                                   |
| [`factory/ext/slogx`](factory/ext/slogx/)                                           | Logger configuration mapped to a go-slogx logger                                                                                                                    |
| [`factory/ext/tlsutils`](factory/ext/tlsutils/)                                     | TLS provider registry and the S3, Vault, and SPIFFE factories                                                                                                       |
| [`observability/appstats`](observability/appstats/)                                 | CPU, memory, network I/O, and goroutine metrics with periodic logging                                                                                               |
| [`observability/health`](observability/health/)                                     | Health check coordinator with subscriptions                                                                                                                         |
| [`observability/metrics`](observability/metrics/)                                   | Prometheus metrics via interface-driven adapters                                                                                                                    |
| [`observability/tracing`](observability/tracing/)                                   | Distributed tracing (OpenTelemetry, OTLP, console) with samplers                                                                                                    |
| [`proto`](proto/)                                                                   | Protobuf definitions and generated Go code for gRPC services                                                                                                        |
| [`security/vault`](security/vault/)                                                 | HashiCorp Vault client (AppRole, token, userpass auth)                                                                                                              |
| [`messaging/broker`](messaging/broker/)                                             | Message broker abstraction (NATS JetStream, outbox)                                                                                                                 |
| [`messaging/broker/idempotency`](messaging/broker/idempotency/)                     | Consumer deduplication middleware over at-least-once broker delivery                                                                                                |
| [`transport/grpc`](transport/grpc/)                                                 | gRPC server, interceptors, handlers, gRPC client                                                                                                                    |
| [`transport/grpc/handlers/scheduler`](transport/grpc/handlers/scheduler/)           | gRPC `SchedulerService` over a go-horai scheduler: CEL filters and signed page tokens                                                                               |
| [`transport/http`](transport/http/)                                                 | HTTP server, middlewares, codec registry, HTTP client                                                                                                               |

## Command-line tools

The configuration converter `goconfig` lives in [go-nomos](https://github.com/altessa-s/go-nomos/tree/develop/cmd/goconfig). The functional options
generator lives in [go-optgen](https://github.com/altessa-s/go-optgen); this module runs it as a Go tool (`go tool optgen`, pinned by the `tool`
directive in `go.mod`).

## Documentation

| Guide                                              | Description                                                                                                                       |
| -------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| [Architecture](docs/architecture.md)               | Package structure, layering, and design principles                                                                                |
| [Coordination & Consistency](docs/coordination.md) | Choosing between `eventbus`, `uow`, `outbox`, `broker`, and `saga`; the dual-write problem; how they compose                      |
| [Saga](docs/reliability/saga.md)                   | Saga orchestration: model, status lifecycle, crash recovery, diagrams, and examples                                               |
| [Configuration](docs/configuration.md)             | Multi-source config loading, env vars, secrets                                                                                    |
| [Self-Issued JWT](docs/auth/selfjwt.md)            | Per-subject JWT minting and fail-closed verification, key rotation, verification-key cache                                        |
| [Static Tokens](docs/auth/static.md)               | API-key / pre-shared token validation, HMAC-SHA256 storage, failure-only rate limiting                                            |
| [OPA Authorization](docs/auth/opa.md)              | Rego policy evaluation, pluggable sources (embed/fs/GitLab/S3), atomic hot-reload, events                                         |
| [Health](docs/observability/health.md)             | Coordinator, HTTP probes (`/healthz`, `/readyz`), gRPC `grpc_health_v1`                                                           |
| [Proxy](docs/proxy.md)                             | Outbound HTTP/gRPC proxy: YAML modes, wiring, TLS to proxy                                                                        |
| [Metrics Reference](docs/metrics.md)               | Prometheus metrics across toolkit subsystems                                                                                      |
| [Event Bus](docs/messaging/eventbus.md)            | In-process event bus: delivery & transaction semantics, the `uow` post-commit compensation companion, pluggable database backends |

### Engineering guides

Opinionated engineering guidance for services built on the toolkit: general practice plus the team's default choices.

| Guide                                                                       | Description                                                                                                            |
| --------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| [Concurrency and Consistency Patterns](docs/guides/concurrency-patterns.md) | In-process concurrency, database-level locking, distributed coordination, outbox/idempotency/saga, overload protection |
| [Message Brokers and Queues](docs/guides/message-brokers.md)                | Choosing a broker, delivery semantics and ordering, NATS/Kafka/RabbitMQ and alternatives, reliability patterns         |
| [Observability](docs/guides/observability.md)                               | Correlation, metrics, logs, tracing, alerting and SLOs, dashboards, probes, new-service checklist                      |
| [Performance and Profiling](docs/guides/performance-profiling.md)           | Latency metrics, pprof, execution traces, benchmarks, GC tuning, PGO, load testing, investigation checklist            |

Full API documentation is available at [pkg.go.dev](https://pkg.go.dev/github.com/altessa-s/go-atlas).

## Development

```bash
git clone https://github.com/altessa-s/go-atlas.git
cd go-atlas
```

| Command              | Description                                                                                                              |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `make fmt`           | Format code and sort imports                                                                                             |
| `make lint`          | Run golangci-lint with the repo config (`.golangci.yml`)                                                                 |
| `make test`          | Run root-module tests with default build tags; integration and stress tests have separate targets                        |
| `make test-all`      | Tests with race detector, shuffle, and coverage                                                                          |
| `make test-stress`   | Stress tests under `-race`, `STRESS_DURATION` (default 30s) each                                                         |
| `make bench`         | Run all benchmarks                                                                                                       |
| `make bench-gate`    | Fail on hot-path benchmark regressions against the local baseline ([details](CONTRIBUTING.md#benchmark-regression-gate)) |
| `make security-scan` | Run `govulncheck` and `gosec`                                                                                            |
| `make generate`      | Run `go generate` across all packages                                                                                    |
| `make ci`            | Run full CI checks locally (lint, test, security, dupl)                                                                  |
| `make clean`         | Remove generated artifacts                                                                                               |

Stress tests live next to the code they exercise in `*_stress_test.go` files behind the `stress` build tag, so `go test ./...` never runs them. Each one
mixes every operation of its package concurrently for `ATLAS_STRESS_DURATION` (default 30s) and checks invariants as it goes; run one with
`ATLAS_STRESS_DURATION=10s go test -tags stress -race -run '^TestStress' ./coord/limiters/storages/memory`. The dlock test needs the NATS broker from
`make integration-up` (`NATS_URL` overrides it) and skips without one. The `Stress` workflow runs them nightly.

## Contributing

Contributions are welcome. Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Security

Report vulnerabilities by email to **security@altessa-s.com**. See [SECURITY.md](SECURITY.md).

## License

MIT License — Copyright (c) 2021-2026 [ALTESSA SOLUTIONS INC](https://altessa-s.com). See [LICENSE](LICENSE).
