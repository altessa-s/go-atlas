# auth

Authentication and authorization subsystem for the Atlas framework. Provides a low-level JWT signing/verification toolkit, OpenID Connect
token validation, self-issued JWT minting and verification, scope-based access control, Open Policy Agent integration, and an
authorization-decision audit trail — all with pluggable backends and automatic lifecycle management.

## Packages

| Package              | Description                                                                 |
|----------------------|-----------------------------------------------------------------------------|
| [audit](./audit)     | Authorization-decision audit trail: `Decision` + consumer-side `Sink` seam + `Recorder` with policy/failure modes |
| [jwt](./jwt)         | Low-level JWT signing/verification toolkit (Signer/Verifier/Claims) over golang-jwt; shared core for selfjwt and oidc |
| [oidc](./oidc)       | OIDC JWT validation with JWKS rotation, presets, and introspection          |
| [opa](./opa)         | OPA policy evaluation with hot-reload and event-driven architecture         |
| [scope](./scope)     | Transport-neutral, deny-by-default scope-based authorization (core + gRPC/HTTP adapters) |
| [selfjwt](./selfjwt) | Self-issued JWT minting and verification with per-subject keys and rotation; `New` builds a matched minter/verifier pair |
| [mtls](./mtls)       | Verified mTLS client cert → principal core: identity + validators (expiry, trust-domain, revocation) + audit; behind the gRPC/HTTP mTLS adapters |
| [spiffe](./spiffe)   | SPIFFE ID parsing from X.509 certificates (trust domain + path); pure primitive behind the mTLS interceptor |
| [static](./static)   | Static token / API-key validator for service-to-service auth               |

## Design principles

- **Standards-based** — OIDC discovery (RFC 8414), token introspection (RFC 7662), and OPA Rego policies.
- **Secure by default** — asymmetric-only signing algorithms, revocation checks, checksum-verified policy files.
- **Factory-driven** — each subsystem includes a `factory` subpackage for configuration-based setup.
- **Hot-reload** — JWKS key rotation and filesystem policy watching keep authorization current without restarts.
- **Auditable** — every authn/authz decision can be recorded through the `audit` primitive without coupling the deciding engines to storage.
