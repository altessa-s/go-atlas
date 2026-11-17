# transport

Transport layer for the Atlas framework. Provides gRPC and HTTP abstractions (the message broker lives in [messaging](../messaging)) with production-ready features including resilience,
observability, security, and configuration-driven creation of all transport components.

## Design principles

- **Provider pattern** — transport-specific implementations (NATS, http.ServeMux) are pluggable behind stable interfaces
- **Driven interceptor/middleware** — separates lifecycle hooks from protocol-specific signatures for reusable cross-cutting concerns
- **Dependency ordering** — interceptor and middleware chains resolve execution order automatically via topological sort
- **Configuration-driven** — factory packages create fully wired components from structured config objects

## Subpackages

| Package                                                                       | Description                                                                                           |
|-------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------|
| [clientip](https://github.com/altessa-s/go-atlas-net/tree/develop/clientip)   | Real client IP extraction behind trusted proxies, shared by the HTTP and gRPC realip components       |
| [grpc](./grpc)                                                                | gRPC server and client with connection pooling, interceptor chain, and service handlers               |
| [http](./http)                                                                | HTTP server and client with content negotiation, middleware chain, and pluggable router               |
| [proxydial](https://github.com/altessa-s/go-atlas-net/tree/develop/proxydial) | Forward-proxy dialers for non-HTTP client connections (SMTP, SOAP, raw TCP, gRPC over CONNECT/SOCKS5) |
