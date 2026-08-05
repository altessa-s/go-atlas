# oauth2client

```go
import "github.com/altessa-s/go-atlas/auth/oauth2client"
```

Acquires OAuth2 tokens from an external identity provider as a **client**. It is the acquisition counterpart to `auth/oidc` (which
*verifies* inbound tokens) and `auth/selfjwt` (which *mints* self-issued tokens): this package *fetches* tokens for outbound,
service-to-service calls.

Every helper returns a `golang.org/x/oauth2.TokenSource` — the same type consumed by
[`transport/grpc/client.NewInsecureTokenCredentials`](../../transport/grpc/client) and
[`oidc.Provider.UserInfo`](../oidc) — so an acquired token drops straight into the transport layer with no adapter.

The three x/oauth2-native grants are thin wrappers that add consistent options and a self-refreshing source; RFC 8693 token exchange
has no x/oauth2 equivalent and is implemented here as a spec-compliant client.

## Grants

| Grant                | Entry point                                    | RFC          | Use for                                                     |
|----------------------|------------------------------------------------|--------------|-------------------------------------------------------------|
| `client_credentials` | `ClientCredentials(ctx, tokenURL, id, secret)` | 6749 §4.4    | Machine-to-machine; the service authenticates as itself.    |
| `refresh_token`      | `Refresh(ctx, tokenURL, id, secret, refresh)`  | 6749 §6      | Resume a session from a stored refresh token.               |
| `authorization_code` | `NewAuthCode(endpoint, id, secret, redirect)`  | 6749 §4.1    | Human consent: build the URL, then exchange the code.       |
| token exchange       | `NewExchanger(tokenURL, id, secret)`           | 8693         | Trade a subject token for one scoped to a downstream target.|
| device code          | `NewDeviceFlow(endpoint, id, secret)`          | 8628         | Input-constrained clients (CLI, TV): show a code, poll for the token. |

## Options

| Option                       | Applies to                          | Description                                                              |
|------------------------------|-------------------------------------|-------------------------------------------------------------------------|
| `WithScopes(...string)`      | all                                 | Requested scopes; trimmed and de-duplicated. `AuthCode` reads them too. |
| `WithAuthStyle(oauth2.AuthStyle)` | `ClientCredentials`, `Refresh`, `Exchanger` | Present credentials in the header or the body. Default: auto-detect. |
| `WithEndpointParams(url.Values)`  | `ClientCredentials`, `Exchanger` | Extra non-standard token params (e.g. Auth0 `audience`).           |
| `WithHTTPClient(*http.Client)`    | all                             | Custom client for timeouts, mTLS, or tracing round-trippers.           |
| `WithRetryAttempts(int)`          | `Exchanger`                     | Retries after the first attempt on transport errors, 429, and 5xx. 0 (default) disables. |
| `WithRetryBaseDelay` / `WithRetryMaxDelay` | `Exchanger`            | Bound the exponential backoff between token-exchange retries.           |
| `WithMetrics(*Metrics)`           | all                             | Record fetch counts, latency, and retries. Build with `NewMetrics`.    |
| `WithLogger(*slog.Logger)`        | all                             | Log token-fetch failures and retry attempts. Nil (default) disables.   |
| `WithClientAuth(ClientAuthenticator)` | `ClientCredentials`, `Exchanger` | Authenticate with a signed JWT assertion (RFC 7523), superseding the secret. |
| `WithEarlyExpiry(time.Duration)`  | `ClientCredentials`, `Exchanger.TokenSource` | Refresh this long before exp (clock-skew margin). 0 = x/oauth2 default (~10s). |

## Key types

| Type              | Description                                                                                       |
|-------------------|---------------------------------------------------------------------------------------------------|
| `AuthCode`        | Authorization-code driver: `AuthCodeURL(state, ...)` then `Exchange(ctx, code, ...)`.             |
| `Exchanger`       | RFC 8693 client: `Exchange(ctx, ExchangeRequest)` and self-refreshing `TokenSource(ctx, req)`.   |
| `ExchangeRequest` | One exchange: `SubjectToken` (required), optional actor token, `Audience`/`Resource`, `Scopes`.  |

## Usage

Machine-to-machine, wired into an outbound gRPC client:

```go
src := oauth2client.ClientCredentials(ctx, tokenURL, clientID, clientSecret,
    oauth2client.WithScopes("orders:read", "orders:write"),
)
creds := client.NewInsecureTokenCredentials(src) // every call now carries a fresh token
```

Token exchange, propagating the caller's identity to a downstream service (RFC 8693):

```go
ex := oauth2client.NewExchanger(tokenURL, clientID, clientSecret)
tok, err := ex.Exchange(ctx, oauth2client.ExchangeRequest{
    SubjectToken: inboundAccessToken,
    Audience:     "https://downstream.internal",
})
```

## Discovery, Principal, and config

- **OIDC discovery** — `ClientCredentialsFromDiscovery` / `NewExchangerFromDiscovery` take the token endpoint from a
  `TokenEndpointSource` (a narrow `interface{ TokenEndpoint() string }` satisfied by [`oidc.Provider`](../oidc)) instead of a literal
  URL, so acquisition and verification point at the same IdP metadata. They return `ErrNoTokenEndpoint` if discovery has not resolved
  one.
- **Principal** — `Principal(subject, tok)` builds an [`auth/principal.Principal`](../principal) from a fetched token (subject + the
  granted `scope`), the same type the scope enforcer consumes. It does not verify the token.
- **Config + factory** — [`config.OAuth2Client`](../../config) + [`factory`](factory) build a ready client_credentials
  `oauth2.TokenSource` (or `Exchanger`) from a YAML template, resolving the endpoint from `tokenUrl` or OIDC `discoveryUrl`.

## Client authentication (JWT assertion)

Enterprise / FAPI IdPs often require the client to authenticate with a signed JWT assertion (RFC 7523) instead of a shared secret.
`WithClientAuth` supplies one; it supersedes `WithAuthStyle`/`clientSecret` for `ClientCredentials` and `Exchanger` (the assertion is
minted fresh per fetch — `iss`=`sub`=client id, `aud`=token endpoint, unique `jti`, short `exp`). `Refresh` and `AuthCode` keep the
secret path.

| Constructor                                | Scheme             | Signs the assertion with                          |
|--------------------------------------------|--------------------|---------------------------------------------------|
| `PrivateKeyJWT(clientID, jwt.SigningKey)`  | `private_key_jwt`  | the client's asymmetric private key (kid → IdP).  |
| `ClientSecretJWT(clientID, clientSecret)`  | `client_secret_jwt`| the client secret via HS256 (secret never sent).  |

```go
auth, _ := oauth2client.PrivateKeyJWT("svc", jwt.SigningKey{KeyID: "key-1", Algorithm: jwt.AlgRS256, Key: privKey})
src := oauth2client.ClientCredentials(ctx, tokenURL, "svc", "", oauth2client.WithClientAuth(auth))
```

Tune the assertion with `WithAssertionLifetime` and `WithAssertionAudience`. Via YAML, set the [`config.OAuth2Client`](../../config)
`clientAuth` block (`method` + PEM `privateKey`/`keyId`/`algorithm`) and the [factory](factory) builds the authenticator for you.

## Refresh timing and revocation

- **Early refresh** — `WithEarlyExpiry(d)` refreshes a token `d` before its `exp` to absorb clock skew and in-flight latency (for
  `ClientCredentials` and `Exchanger.TokenSource`). `Refresh`/`AuthCode` keep x/oauth2's default window — overriding it there would
  drop refresh-token rotation.
- **Revocation** — `NewRevoker(revocationURL, id, secret).Revoke(ctx, token, WithTokenTypeHint(...))` revokes an acquired token at the
  IdP (RFC 7009), reusing the same client auth (secret or assertion). Per the RFC the endpoint answers 200 even for an unknown token,
  so a nil error means "not (or no longer) valid".

**Retrying the native grants** — only `Exchanger` retries on its own. To add retries to the x/oauth2-backed grants
(`ClientCredentials`, `Refresh`, `AuthCode`, `DeviceFlow`), wrap the transport with `RetryTransport` and inject it via
`WithHTTPClient`; it retries transport errors, 429, and 5xx (only replayable requests; the last response is surfaced on exhaustion):

```go
client := &http.Client{Transport: oauth2client.RetryTransport(http.DefaultTransport)}
src := oauth2client.ClientCredentials(ctx, tokenURL, id, secret, oauth2client.WithHTTPClient(client))
```

Two RFC edge cases are preserved verbatim (see `parseTokenResponse` godoc): a response without `expires_in` yields a never-expiring
token (a self-refreshing source won't refresh it), and `token_type: "N_A"` (RFC 8693) is not a bearer credential — don't attach it with
`SetAuthHeader`. Client secrets are held for the source lifetime and can't be zeroized; prefer `PrivateKeyJWT` when that matters.

## Observability

`WithMetrics(NewMetrics(collector, ""))` records, under subsystem `auth_oauth2client`:

| Metric                              | Type      | Labels           | Meaning                                             |
|-------------------------------------|-----------|------------------|-----------------------------------------------------|
| `token_fetches_total`               | counter   | `grant`, `status`| Token acquisitions (cache hits are **not** counted).|
| `token_fetch_duration_seconds`      | histogram | `grant`, `status`| Acquisition latency.                                |
| `token_fetch_retries_total`         | counter   | `grant`          | Retries after a transient token-exchange failure.   |

`grant` is one of `client_credentials` / `refresh_token` / `authorization_code` / `token_exchange`. A metrics-wrapped source records
only genuine acquisitions — when the reuse cache serves an unexpired token, nothing is recorded. `WithLogger` logs fetch failures and
retry attempts at warn level (never the token itself). Both default to off, keeping the hot path allocation-free.

## Errors

`ClientCredentials`, `Refresh`, and `AuthCode` surface the underlying x/oauth2 error unchanged (typically an
`*oauth2.RetrieveError`, which carries the IdP status and body). `Exchanger` returns `ErrSubjectTokenRequired` for a missing subject
token and wraps `ErrTokenExchange` for a transport failure, a non-2xx response, or an unparsable body — match both with `errors.Is`.

## See also

- [`auth/oidc`](../oidc) — verify inbound tokens; consumes the same `oauth2.TokenSource` for userinfo.
- [`auth/selfjwt`](../selfjwt) — mint self-issued tokens.
- [`transport/grpc/client`](../../transport/grpc/client) — `NewInsecureTokenCredentials` attaches a `TokenSource` to gRPC calls.
