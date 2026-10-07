# authconfig

```go
import authconfig "github.com/altessa-s/go-atlas/config/auth"
```

Package `authconfig` defines authentication and authorization schemas: OIDC, OPA, mTLS, scopes, denylist, OAuth2 client, SPIFFE and the aggregate Auth
block. Schemas are populated by [`config/loader`](../loader) and consumed by the component factories, which map them to generated options; runtime
packages never import them.

## Key types

| Type                    | Description                                                                                                                                                                                  |
|-------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Config`                  | Is the combined authentication and authorization configuration: OIDC and mutual-TLS for authentication, OPA and the scope registry for authorization.                                        |
| `Denylist`              | Is the configuration for the token-revocation denylist with a negative-cache in front of a distributed authoritative store.                                                                  |
| `MTLS`                  | Is the configuration for the mutual-TLS authentication validators built by [auth/mtls/factory.New].                                                                                          |
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
| `ScopeRegistry`         | Is the configuration for a scope authorization registry: the declarative action-key→required-scope table consumed by [auth/scope/factory.New].                                               |
| `OAuth2Client`          | Configures client-side OAuth2 token acquisition from an external identity provider (the acquisition counterpart to the inbound-facing OIDC config).                                          |
| `OAuth2ClientRetry`     | Configures retrying of token exchanges with exponential backoff.                                                                                                                             |
| `OAuth2ClientAuth`      | Configures JWT-assertion client authentication (RFC 7523): private_key_jwt (a signed assertion) or client_secret_jwt (HMAC over the clientSecret).                                           |
| `OPASourceProvider`     | Defines the type of OPA policy source.                                                                                                                                                       |
| `OPACache`              | Represents caching configuration for OPA authorization decisions.                                                                                                                            |
| `OPAGitLab`             | Represents GitLab-specific configuration for the OPA policy source.                                                                                                                          |
| `OPAS3`                 | Represents S3-specific configuration for the OPA policy source.                                                                                                                              |
| `OPA`                   | Represents Open Policy Agent configuration for authorization.                                                                                                                                |
| `SPIFFE`                | Configures the SPIFFE Workload API source built by [security/tlsutils/spiffe/factory.New]: the Workload API endpoint plus the accepted peer identities that the derived authorizer enforces. |

See the [config index](../README.md) for the other schema packages.
