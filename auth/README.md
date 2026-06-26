# auth

Authentication and authorization subsystem for the Atlas framework. Provides OpenID Connect token validation, self-issued JWT minting and
verification, and Open Policy Agent integration for policy-based access control — all with pluggable backends and automatic lifecycle
management.

## Packages

| Package              | Description                                                                 |
|----------------------|-----------------------------------------------------------------------------|
| [oidc](./oidc)       | OIDC JWT validation with JWKS rotation, presets, and introspection          |
| [opa](./opa)         | OPA policy evaluation with hot-reload and event-driven architecture         |
| [selfjwt](./selfjwt) | Self-issued JWT minting and verification with per-subject keys and rotation |
| [static](./static)   | Static token / API-key validator for service-to-service auth               |

## Design principles

- **Standards-based** — OIDC discovery (RFC 8414), token introspection (RFC 7662), and OPA Rego policies.
- **Secure by default** — asymmetric-only signing algorithms, revocation checks, checksum-verified policy files.
- **Factory-driven** — each subsystem includes a `factory` subpackage for configuration-based setup.
- **Hot-reload** — JWKS key rotation and filesystem policy watching keep authorization current without restarts.
