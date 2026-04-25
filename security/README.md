# security

Security subsystem for the Atlas framework. Provides secret management, TLS certificate handling, and HashiCorp Vault integration — all with
pluggable backends and automatic lifecycle management.

## Packages

| Package                | Description                                                    |
|------------------------|----------------------------------------------------------------|
| [secrets](./secrets)   | Centralized secret management with caching and watch API       |
| [tlsutils](./tlsutils) | TLS certificate loading, OCSP stapling, and secure defaults   |
| [vault](./vault)       | High-level Vault client with pluggable auth and token renewal  |

## Design principles

- **Backend-agnostic** — secret and TLS providers are injected through interfaces.
- **Secure by default** — TLS 1.2+ enforced, secrets cleared on GC, distributed locking.
- **Factory-driven** — each subsystem includes a `factory` subpackage for config-based setup.
- **Generic** — `secrets.Manager[T]` and providers use Go generics for type-safe values.
