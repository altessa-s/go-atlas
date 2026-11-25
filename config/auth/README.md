# authconfig

```go
import authconfig "github.com/altessa-s/go-atlas/config/auth"
```

Package `authconfig` defines authentication and authorization schemas: OIDC, OPA, mTLS, scopes, denylist, OAuth2 client, SPIFFE and the aggregate Auth
block. Schemas are populated by [`go-nomos/loader`](https://github.com/altessa-s/go-nomos/tree/develop/loader) and consumed by the component factories,
which map them to generated options; runtime packages never import them.

`ParseServiceConfig` reads the [OIDC JSON service policy](service-config.schema.json) from bytes and validates the default rules, presets and their
selection conditions. The [OIDC factory](../../factory/auth/oidc/) maps the result to provider options.

Use `errors.Is` with `ErrInvalidVersion`, `ErrInvalidDuration`, or `ErrPresetNotFound` for document-specific failures. The parser and the provider
share [`rules.ErrInvalidConfig` and `rules.ErrInvalidCEL`](../../auth/oidc/rules/) for invalid policies and CEL expressions.

JSON `token_lifetime.min` and `token_lifetime.max` and YAML `minTokenLifetime` and `maxTokenLifetime` bound `exp - iat` in the default policy and
presets. An omitted or zero minimum disables the lower bound. The minimum must be non-negative and at most the enabled maximum.

## Key types

| Type                    | Description                                                                                                                                                                                  |
|-------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Config`                  | Is the combined authentication and authorization configuration: OIDC and mutual-TLS for authentication, OPA and the scope registry for authorization.                                        |
| `Denylist`              | Is the configuration for the token-revocation denylist with a negative-cache in front of a distributed authoritative store.                                                                  |
| `MTLS`                  | Is the configuration for the mutual-TLS authentication validators built by [factory/auth/mtls.New].                                                                                          |
| `OIDC`                  | Represents the configuration for OpenID Connect authentication.                                                                                                                              |
| `OIDCCache`             | Represents token caching configuration.                                                                                                                                                      |
| `OIDCExpression`        | Represents a CEL expression for custom token validation.                                                                                                                                     |
| `OIDCValidation`        | Represents token validation settings for OIDC.                                                                                                                                               |
| `OIDCClaims`            | Represents JWT token claims validation configuration.                                                                                                                                        |
| `OIDCClientCredentials` | Holds OAuth2 client credentials shared across introspection and other server-to-server flows.                                                                                                |
| `OIDCJwks`              | Represents OIDCJwks-specific configuration.                                                                                                                                                  |
| `OIDCIntrospection`     | Controls RFC 7662 token introspection.                                                                                                                                                       |
| `OIDCPresets`           | Represents validation presets configuration.                                                                                                                                                 |
| `OIDCPreset`            | Represents a named validation preset.                                                                                                                                                        |
| `OIDCSelector`          | Represents a rule for automatic preset selection.                                                                                                                                            |
| `OIDCRevocation`        | Represents token or key revocation configuration.                                                                                                                                            |
| `OIDCRevocationSource`  | Represents the source for a revocation list.                                                                                                                                                 |
| `ScopeRule`             | Maps a set of action keys to the scope required to perform them.                                                                                                                             |
| `ScopeRegistry`         | Is the configuration for a scope authorization registry: the declarative action-key→required-scope table consumed by [factory/auth/scope.New].                                               |
| `OAuth2Client`          | Configures client-side OAuth2 token acquisition from an external identity provider (the acquisition counterpart to the inbound-facing OIDC config).                                          |
| `OAuth2ClientRetry`     | Configures retrying of token exchanges with exponential backoff.                                                                                                                             |
| `OAuth2ClientAuth`      | Configures JWT-assertion client authentication (RFC 7523): private_key_jwt (a signed assertion) or client_secret_jwt (HMAC over the clientSecret).                                           |
| `OPASourceProvider`     | Defines the type of OPA policy source.                                                                                                                                                       |
| `OPACache`              | Represents caching configuration for OPA authorization decisions.                                                                                                                            |
| `OPAGitLab`             | Represents GitLab-specific configuration for the OPA policy source.                                                                                                                          |
| `OPAS3`                 | Represents S3-specific configuration for the OPA policy source.                                                                                                                              |
| `OPA`                   | Represents Open Policy Agent configuration for authorization.                                                                                                                                |
| `SPIFFE`                | Configures the SPIFFE Workload API source built by [factory/ext/tlsutils/spiffe.New]: the Workload API endpoint plus the accepted peer identities that the derived authorizer enforces. |

## Secure defaults

The schemas default to the restrictive behavior; each relaxation is an explicit key.

| Schema         | Default                                                                                                              | Opt-out                                    |
|----------------|----------------------------------------------------------------------------------------------------------------------|--------------------------------------------|
| `MTLS`         | A trust-domain decision is required: the factory fails with `mtls.ErrNoTrustDomainPolicy` without `trustDomains`     | `allowAnyTrustDomain: true`                |
| `OAuth2Client` | `tokenURL` / `discoveryURL` must be https; http is accepted on a loopback host only                                  | `allowInsecureHTTP: true`                  |
| `OPAGitLab`    | `endpoint` must be https; without `checksums` the source logs a warning that policies are not integrity-verified     | `allowInsecureHTTP: true`; set `checksums` |
| `OPA`          | `releaseManifest` is required for a filesystem bundle | See the [OPA guide](../../docs/auth/opa.md#publishing-releases) |
| `OPAS3`        | `endpoint` must be HTTPS, and `releaseManifest` or `checksums` is required | `allowInsecureHTTP: true` for plain HTTP |

`OIDCRevocation` bounds and guards the revocation sync with three keys under `oidc.revocation`, next to `maxStaleness`:

| Key                | Type       | Default     | Validation   | Meaning                                                                                      |
|--------------------|------------|-------------|--------------|----------------------------------------------------------------------------------------------|
| `syncTimeout`      | `duration` | `5m`        | non-negative | Time limit of one scheduled sync; `0` disables the timeout                                   |
| `maxResponseBytes` | `int64`    | `268435456` | positive     | Cap on the `source.url` response body in bytes (256 MiB); a larger list fails the sync       |
| `allowEmpty`       | `bool`     | `false`     | —            | Treat an empty list as authoritative; the default fails such a sync and keeps the set        |

`allowEmpty: true` is a relaxation: a source that answers with an empty body during an outage then un-revokes everything. Enable it only when the
revocation list can legitimately become empty.

Some defaults have no key here because they are not configurable from YAML:

- **OCSP revocation** (`auth/mtls/revocation`) is wired in code with `mtls.WithValidator`. It is fail-closed by default, bounds a whole check with a
  5s timeout and follows a redirect only to the responder's own host; `revocation.WithFailMode(revocation.FailOpen)` is the opt-in. Its default
  client refuses a responder on a loopback, private or otherwise non-public address; an internal responder is named with
  `revocation.WithAllowedResponders(hosts…)`, and `revocation.WithInsecureAllowPrivateResponders()` turns the refusal off.
- **OAuth2 client transport.** Without an injected HTTP client the token requests use a client with a 30s timeout that does not follow redirects; IdP
  response bodies, `error_description`, and `error_uri` are withheld from error strings and logs (only the numeric HTTP status, allowlisted RFC error
  codes, and byte lengths are logged).
- **OIDC.** A JWKS key with `use: enc` never verifies a token, and a rejected token is logged at Debug; failures of the provider's own infrastructure
  keep their Error and Warn records.

See the [config index](../README.md) for the other schema packages.
