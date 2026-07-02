# auth

Authentication and authorization subsystem for the Atlas framework. Provides a canonical verified-identity type, a low-level JWT
signing/verification toolkit, OpenID Connect token validation, self-issued JWT minting and verification, client-side OAuth2 token
acquisition for outbound calls, scope-based access control, Open Policy Agent integration, a shared token-revocation denylist, and an
authorization-decision audit trail — all with pluggable backends and automatic lifecycle management.

## Packages

| Package              | Description                                                                 |
|----------------------|-----------------------------------------------------------------------------|
| [principal](./principal) | Canonical verified-identity type (subject/tenant/scopes/roles/raw claims + `FromClaims`); the standard `P` that AuthN adapters produce and scope authorizers consume |
| [jwt](./jwt)         | Low-level JWT signing/verification toolkit (Signer/Verifier/Claims) over golang-jwt; shared core for selfjwt and oidc |
| [oidc](./oidc)       | OIDC JWT validation with JWKS rotation, presets, and introspection          |
| [selfjwt](./selfjwt) | Self-issued JWT minting and verification with per-subject keys and rotation; `New` builds a matched minter/verifier pair |
| [oauth2client](./oauth2client) | Client-side OAuth2 token acquisition for outbound calls: client_credentials / refresh / auth_code / device grants + RFC 8693 exchange, yielding a self-refreshing `oauth2.TokenSource` |
| [static](./static)   | Static token / API-key validator for service-to-service auth               |
| [spiffe](./spiffe)   | SPIFFE ID parsing from X.509 certificates (trust domain + path); pure primitive behind the mTLS interceptor |
| [mtls](./mtls)       | Verified mTLS client cert → principal core: identity + validators (expiry, trust-domain, revocation, subject/issuer/CA-pin, DNS-SAN, EKU) + audit; behind the gRPC/HTTP mTLS adapters |
| [mtls/revocation](./mtls/revocation) | Live OCSP peer-revocation `CertValidator`: queries the issuer's responder, TTL-caches the result, fail-open/closed; network-backed complement to `mtls.RevocationList` |
| [scope](./scope)     | Transport-neutral, deny-by-default scope-based authorization (core + gRPC/HTTP adapters) |
| [opa](./opa)         | OPA policy evaluation with hot-reload and event-driven architecture         |
| [audit](./audit)     | Authorization-decision audit trail: `Decision` + consumer-side `Sink` seam + `Recorder` with policy/failure modes |
| [denylist](./denylist) | Reusable token-revocation seam: concurrency-safe set of revoked ids (jti/subject), permanent or TTL-bounded, exposed as a `Checker`; never evicts a live entry |
| [denylist/negcache](./denylist/negcache) | Probabilistic negative cache (Bloom/Cuckoo filter) in front of an authoritative revocation store, answering never-revoked tokens locally |
| [denylist/storages/redis](./denylist/storages/redis) | Redis-backed authoritative denylist store for cross-instance token revocation |

## Design principles

- **Standards-based** — OIDC discovery (RFC 8414), token introspection (RFC 7662), and OPA Rego policies.
- **Secure by default** — asymmetric-only signing algorithms, revocation checks, checksum-verified policy files.
- **Factory-driven** — each subsystem includes a `factory` subpackage for configuration-based setup.
- **Hot-reload** — JWKS key rotation and filesystem policy watching keep authorization current without restarts.
- **Auditable** — every authn/authz decision can be recorded through the `audit` primitive without coupling the deciding engines to storage.
