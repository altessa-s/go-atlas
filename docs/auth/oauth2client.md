# Client-Side OAuth2 Token Acquisition

The acquisition side of the auth stack: this service fetching an access token to call another. It is the mirror of `oidc` (which *verifies*
inbound tokens) and `selfjwt` (which *mints* self-issued ones). Every helper returns a self-refreshing `oauth2.TokenSource`, the same
currency the gRPC client credentials consume, so an acquired token drops into the transport layer with no adapter.

---

## Table of Contents

- [Overview](#overview)
- [Grants](#grants)
- [Options](#options)
- [Quick Start](#quick-start)
  - [Machine-to-machine (client_credentials)](#machine-to-machine-client_credentials)
  - [Token exchange (RFC 8693)](#token-exchange-rfc-8693)
  - [Discovery from an OIDC provider](#discovery-from-an-oidc-provider)
  - [Config + factory](#config--factory)
- [Client Authentication (JWT assertion)](#client-authentication-jwt-assertion)
- [Refresh, Retry, and Revocation](#refresh-retry-and-revocation)
- [Observability](#observability)
- [Errors](#errors)
- [API Reference](#api-reference)
- [Design Notes](#design-notes)
- [See Also](#see-also)

## Overview

`auth/oauth2client` acquires OAuth2 tokens from an external identity provider *as a client*. The three x/oauth2-native grants
(client_credentials, refresh_token, authorization_code) are thin wrappers that add consistent `Option` configuration and return a
self-refreshing source; the device-code grant (RFC 8628) and RFC 8693 token exchange — which have no x/oauth2 equivalent — are implemented
here as spec-compliant clients.

```go
import "github.com/altessa-s/go-atlas/auth/oauth2client"
```

Because every helper yields an `oauth2.TokenSource`, the result flows straight into
`transport/grpc/client.NewInsecureTokenCredentials` or `oidc.Provider.UserInfo` without any glue.

## Grants

| Grant                | Entry point                                    | RFC       | Use for                                                      |
|----------------------|------------------------------------------------|-----------|--------------------------------------------------------------|
| `client_credentials` | `ClientCredentials(ctx, tokenURL, id, secret)` | 6749 §4.4 | Machine-to-machine; the service authenticates as itself.     |
| `refresh_token`      | `Refresh(ctx, tokenURL, id, secret, refresh)`  | 6749 §6   | Resume a session from a stored refresh token.                |
| `authorization_code` | `NewAuthCode(endpoint, id, secret, redirect)`  | 6749 §4.1 | Human consent: build the URL, then exchange the code.        |
| token exchange       | `NewExchanger(tokenURL, id, secret)`           | 8693      | Trade a subject token for one scoped to a downstream target. |
| device code          | `NewDeviceFlow(endpoint, id, secret)`          | 8628      | Input-constrained clients (CLI, TV): show a code, poll.      |

## Options

| Option                                     | Applies to                                    | Effect                                                                 |
|--------------------------------------------|-----------------------------------------------|-----------------------------------------------------------------------|
| `WithScopes(s…)`                           | all                                           | Requested scopes; trimmed and de-duplicated.                          |
| `WithAuthStyle(style)`                     | `ClientCredentials`, `Refresh`, `Exchanger`   | Present credentials in the header or the body. Default: auto-detect.  |
| `WithEndpointParams(v)`                    | `ClientCredentials`, `Exchanger`              | Extra non-standard token params (e.g. Auth0 `audience`).              |
| `WithHttpClient(c)`                        | all                                           | Custom client for timeouts, mTLS, or tracing round-trippers.          |
| `WithClientAuth(a)`                        | `ClientCredentials`, `Exchanger`              | Authenticate with a signed JWT assertion (RFC 7523), superseding the secret. |
| `WithEarlyExpiry(d)`                       | `ClientCredentials`, `Exchanger.TokenSource`  | Refresh `d` before `exp` (clock-skew margin). 0 = x/oauth2 default (~10s). |
| `WithRetryAttempts(n)`                     | `Exchanger`                                   | Retries after the first attempt on transport errors, 429, 5xx. 0 disables. |
| `WithRetryBaseDelay` / `WithRetryMaxDelay` | `Exchanger`                                   | Bound the exponential backoff between exchange retries.               |
| `WithMetrics(m)`                           | all                                           | Record fetch counts, latency, retries. Build with `NewMetrics`.       |
| `WithLogger(l)`                            | all                                           | Log fetch failures and retry attempts (never the token). Nil disables. |

## Quick Start

### Machine-to-machine (client_credentials)

`ClientCredentials` returns a source that fetches on first use and refreshes as the token nears expiry.

```go
import (
    "github.com/altessa-s/go-atlas/auth/oauth2client"
    "github.com/altessa-s/go-atlas/transport/grpc/client"
)

src := oauth2client.ClientCredentials(ctx, tokenURL, clientID, clientSecret,
    oauth2client.WithScopes("orders:read", "orders:write"),
)
creds := client.NewInsecureTokenCredentials(src) // every outbound call now carries a fresh token
```

### Token exchange (RFC 8693)

Trade an inbound token for one scoped to a downstream service, propagating the caller's identity. `Exchanger.TokenSource(ctx, req)` wraps a
repeated exchange in a self-refreshing source; `Exchange` is the one-shot form.

```go
ex := oauth2client.NewExchanger(tokenURL, clientID, clientSecret)
tok, err := ex.Exchange(ctx, oauth2client.ExchangeRequest{
    SubjectToken: inboundAccessToken,          // required
    Audience:     "https://downstream.internal",
    // optional: ActorToken for delegation, Resource, Scopes
})
```

### Discovery from an OIDC provider

`ClientCredentialsFromDiscovery` / `NewExchangerFromDiscovery` take the token endpoint from a `TokenEndpointSource` — a narrow
`interface{ TokenEndpoint() string }` satisfied by [`oidc.Provider`](oidc.md) — so acquisition and verification point at the same IdP
metadata. They return `ErrNoTokenEndpoint` if discovery has not resolved one.

```go
src, err := oauth2client.ClientCredentialsFromDiscovery(ctx, provider, clientID, clientSecret,
    oauth2client.WithScopes("orders:read"))
```

### Config + factory

Load a ready client_credentials source (or an `Exchanger`) from `config.OAuth2Client`. The endpoint comes from `tokenUrl`, or — when the
config sets `discoveryUrl` — from an injected OIDC discovery source.

```go
import "github.com/altessa-s/go-atlas/auth/oauth2client/factory"

src, err := factory.New(cfg.OAuth2Client).
    UseTokenEndpointSource(provider). // needed only when config uses discoveryUrl
    Build(ctx)
```

## Client Authentication (JWT assertion)

Enterprise / FAPI IdPs often require the client to authenticate with a signed JWT assertion (RFC 7523) instead of a shared secret.
`WithClientAuth` supplies one; it supersedes `WithAuthStyle`/`clientSecret` for `ClientCredentials` and `Exchanger` (the assertion is minted
fresh per fetch — `iss`=`sub`=client id, `aud`=token endpoint, unique `jti`, short `exp`). `Refresh` and `AuthCode` keep the secret path.

| Constructor                               | Scheme              | Signs the assertion with                          |
|-------------------------------------------|---------------------|---------------------------------------------------|
| `PrivateKeyJWT(clientID, jwt.SigningKey)` | `private_key_jwt`   | the client's asymmetric private key (kid → IdP).  |
| `ClientSecretJWT(clientID, clientSecret)` | `client_secret_jwt` | the client secret via HS256 (secret never sent).  |

```go
auth, _ := oauth2client.PrivateKeyJWT("svc",
    jwt.SigningKey{KeyID: "key-1", Algorithm: jwt.AlgRS256, Key: privKey})
src := oauth2client.ClientCredentials(ctx, tokenURL, "svc", "", oauth2client.WithClientAuth(auth))
```

Tune the assertion with `WithAssertionLifetime` and `WithAssertionAudience`.

## Refresh, Retry, and Revocation

- **Early refresh.** `WithEarlyExpiry(d)` refreshes a token `d` before its `exp` to absorb clock skew and in-flight latency (for
  `ClientCredentials` and `Exchanger.TokenSource`). `Refresh`/`AuthCode` keep x/oauth2's default window — overriding it there would drop
  refresh-token rotation.
- **Retry.** Only `Exchanger` retries on its own (`WithRetryAttempts`). To add retries to the x/oauth2-backed grants, wrap the transport
  with `RetryTransport` and inject it via `WithHttpClient`; it retries transport errors, 429, and 5xx on replayable requests, surfacing the
  last response on exhaustion.
- **Revocation.** `NewRevoker(revocationURL, id, secret).Revoke(ctx, token, WithTokenTypeHint(…))` revokes an acquired token at the IdP
  (RFC 7009), reusing the same client auth. Per the RFC the endpoint answers `200` even for an unknown token, so a nil error means "not (or
  no longer) valid".

```go
client := &http.Client{Transport: oauth2client.RetryTransport(http.DefaultTransport)}
src := oauth2client.ClientCredentials(ctx, tokenURL, id, secret, oauth2client.WithHttpClient(client))
```

Two RFC edge cases are preserved verbatim: a response without `expires_in` yields a never-expiring token (a self-refreshing source won't
refresh it), and `token_type: "N_A"` (RFC 8693) is not a bearer credential — don't attach it with `SetAuthHeader`.

## Observability

`WithMetrics(NewMetrics(collector, ""))` records, under subsystem `auth_oauth2client`:

| Metric                         | Type      | Labels            | Meaning                                              |
|--------------------------------|-----------|-------------------|------------------------------------------------------|
| `token_fetches_total`          | counter   | `grant`, `status` | Token acquisitions (cache hits are **not** counted). |
| `token_fetch_duration_seconds` | histogram | `grant`, `status` | Acquisition latency.                                 |
| `token_fetch_retries_total`    | counter   | `grant`           | Retries after a transient token-exchange failure.    |

`grant` is one of `client_credentials` / `refresh_token` / `authorization_code` / `token_exchange`. When the reuse cache serves an
unexpired token, nothing is recorded. `WithLogger` logs failures and retries at warn level, never the token. Both default to off, keeping
the hot path allocation-free.

## Errors

`ClientCredentials`, `Refresh`, and `AuthCode` surface the underlying x/oauth2 error unchanged (typically an `*oauth2.RetrieveError`,
carrying the IdP status and body). `Exchanger` returns `ErrSubjectTokenRequired` for a missing subject token and wraps `ErrTokenExchange`
for a transport failure, a non-2xx response, or an unparsable body — match both with `errors.Is`. Discovery-based constructors return
`ErrNoTokenEndpoint` when the endpoint has not resolved.

## API Reference

| Symbol                                              | Description                                                                     |
|-----------------------------------------------------|---------------------------------------------------------------------------------|
| `ClientCredentials(ctx, url, id, secret, opt…)`     | Self-refreshing client_credentials `oauth2.TokenSource`.                        |
| `Refresh(ctx, url, id, secret, refresh, opt…)`      | Source resuming from a stored refresh token.                                    |
| `NewAuthCode(endpoint, id, secret, redirect, opt…)` | Authorization-code driver: `AuthCodeURL` then `Exchange`.                        |
| `NewExchanger(url, id, secret, opt…)`               | RFC 8693 client: `Exchange` and `TokenSource`.                                  |
| `NewDeviceFlow(endpoint, id, secret, opt…)`         | RFC 8628 device-code driver: request a code, then poll.                         |
| `ClientCredentialsFromDiscovery(...)` / `NewExchangerFromDiscovery(...)` | As above, endpoint from a `TokenEndpointSource`.            |
| `NewRevoker(url, id, secret, opt…)`                 | RFC 7009 revocation client; `Revoke(ctx, token, …)`.                            |
| `RetryTransport(rt)`                                | `http.RoundTripper` retrying transport errors, 429, 5xx on replayable requests. |
| `PrivateKeyJWT(id, key)` / `ClientSecretJWT(id, secret)` | RFC 7523 client authenticators for `WithClientAuth`.                       |
| `Principal(subject, tok)`                           | Build an `auth/principal.Principal` from a fetched token (does not verify it).   |
| `ExchangeRequest`                                   | One exchange: `SubjectToken` (required), actor token, `Audience`/`Resource`, `Scopes`. |
| `ErrTokenExchange` / `ErrSubjectTokenRequired` / `ErrNoTokenEndpoint` | Sentinel errors matched with `errors.Is`.                      |
| `factory.New(cfg).Build(ctx)` / `.BuildExchanger(ctx)` | Build a source / `Exchanger` from `config.OAuth2Client`.                     |

## Design Notes

- **Acquisition, not verification.** This package never validates tokens — it fetches them. Verification is [`oidc`](oidc.md)'s job; the two
  meet at OIDC discovery, where both read the same IdP metadata.
- **`TokenSource` as the contract.** Returning x/oauth2's `TokenSource` (not a bespoke type) means acquired tokens compose with every
  x/oauth2-aware consumer, including the framework's gRPC client credentials, for free.
- **Secrets.** Client secrets are held for the source's lifetime (presented on every refresh) and cannot be zeroized. Deployments that must
  avoid a long-lived in-memory secret should prefer `PrivateKeyJWT`, where the caller manages the private key.

## See Also

- [`auth/oauth2client` README](../../auth/oauth2client/README.md) — package quick reference.
- [oidc.md](oidc.md) — verify inbound tokens; the discovery source these constructors can share.
- [selfjwt.md](selfjwt.md) — mint self-issued tokens (the other producing side).
- [principal.md](principal.md) — the identity type `Principal(subject, tok)` builds.
- [architecture.md](../architecture.md) — package map and layering.
