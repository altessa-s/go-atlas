# transport

Transport layer for the Atlas framework. Provides message broker, gRPC, and HTTP abstractions with production-ready features including
resilience, observability, security, and configuration-driven creation of all transport components.

## Design principles

- **Provider pattern** — transport-specific implementations (NATS, gorilla/mux, http.ServeMux) are pluggable behind stable interfaces
- **Driven interceptor/middleware** — separates lifecycle hooks from protocol-specific signatures for reusable cross-cutting concerns
- **Dependency ordering** — interceptor and middleware chains resolve execution order automatically via topological sort
- **Configuration-driven** — factory packages create fully wired components from structured config objects

## Subpackages

| Package                | Description                                                                                        |
|------------------------|----------------------------------------------------------------------------------------------------|
| [broker](./broker)     | High-level message broker with Transactional Outbox pattern and NATS JetStream provider            |
| [grpc](./grpc)         | gRPC server and client with connection pooling, interceptor chain, and service handlers            |
| [http](./http)         | HTTP server and client with content negotiation, middleware chain, and pluggable router             |
