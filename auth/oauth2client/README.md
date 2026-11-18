# oauth2client

```go
import "github.com/altessa-s/go-atlas/auth/oauth2client"
```

Acquires OAuth2 tokens from an external identity provider as a **client**. It is the acquisition counterpart to `auth/oidc` (which *verifies* inbound
tokens) and `auth/selfjwt` (which *mints* self-issued tokens): this package *fetches* tokens for outbound, service-to-service calls.

Every helper returns a `golang.org/x/oauth2.TokenSource` — the same type consumed by
[`transport/grpc/client.NewInsecureTokenCredentials`](../../transport/grpc/client) and [`oidc.Provider.UserInfo`](../oidc) — so an acquired token
drops
straight into the transport layer with no adapter.

The three x/oauth2-native grants are thin wrappers that add consistent options and a self-refreshing source; RFC 8693 token exchange has no x/oauth2
equivalent and is implemented here as a spec-compliant client.

## Grants

| Grant                | Entry point                                    | RFC       | Use for                                                               |
|----------------------|------------------------------------------------|-----------|-----------------------------------------------------------------------|
| `client_credentials` | `ClientCredentials(ctx, tokenURL, id, secret)` | 6749 §4.4 | Machine-to-machine; the service authenticates as itself.              |
| `refresh_token`      | `Refresh(ctx, tokenURL, id, secret, refresh)`  | 6749 §6   | Resume a session from a stored refresh token.                         |
| `authorization_code` | `NewAuthCode(endpoint, id, secret, redirect)`  | 6749 §4.1 | Human consent: build the URL, then exchange the code.                 |
| token exchange       | `NewExchanger(tokenURL, id, secret)`           | 8693      | Trade a subject token for one scoped to a downstream target.          |
| device code          | `NewDeviceFlow(endpoint, id, secret)`          | 8628      | Input-constrained clients (CLI, TV): show a code, poll for the token. |
| JWT bearer           | `NewJWTBearer(tokenURL, issuer, key)`          | 7523 §2.1 | Signed-assertion grant (Google service accounts); context-bound.      |

## Options

| Option                                     | Applies to                                                | Description                                                                                        |
|--------------------------------------------|-----------------------------------------------------------|----------------------------------------------------------------------------------------------------|
| `WithScopes(...string)`                    | all                                                       | Requested scopes; trimmed and de-duplicated. `AuthCode` reads them too.                            |
| `WithAuthStyle(oauth2.AuthStyle)`          | `ClientCredentials`, `Refresh`, `Exchanger`               | Present credentials in the header or the body. Default: auto-detect.                               |
| `WithEndpointParams(url.Values)`           | `ClientCredentials`, `Exchanger`                          | Extra non-standard token params (e.g. Auth0 `audience`).                                           |
| `WithHTTPClient(*http.Client)`             | all                                                       | Custom client for timeouts, mTLS, or tracing round-trippers. Default: built-in client (see below). |
| `WithInsecureHTTP()`                       | all                                                       | Allow plain-http endpoints on any host. Default: https, or http on a loopback host only.           |
| `WithRetryAttempts(int)`                   | `Exchanger`, `JWTBearer`                                  | Retries after the first attempt on transport errors, 429, and 5xx. 0 (default) disables.           |
| `WithRetryBaseDelay` / `WithRetryMaxDelay` | `Exchanger`, `JWTBearer`                                  | Bound the exponential backoff between token-exchange retries.                                      |
| `WithMetrics(*Metrics)`                    | all                                                       | Record fetch counts, latency, and retries. Build with `NewMetrics`.                                |
| `WithLogger(*slog.Logger)`                 | all                                                       | Log token-fetch failures and retry attempts. Nil (default) disables.                               |
| `WithClientAuth(ClientAuthenticator)`      | `ClientCredentials`, `Exchanger`                          | Authenticate with a signed JWT assertion (RFC 7523), superseding the secret.                       |
| `WithEarlyExpiry(time.Duration)`           | `ClientCredentials`, `Exchanger.TokenSource`, `JWTBearer` | Refresh this long before exp (clock-skew margin). 0 = x/oauth2 default (~10s).                     |
| `WithClock(clock.Clock)`                   | `JWTBearer`                                               | Time source for assertion `iat`/`exp`, token expiry, and cache freshness. Default: `clock.Real()`. |

## Transport security

- **Built-in client** — without `WithHTTPClient` every flow, the x/oauth2-backed ones included, uses a client with a `DefaultHTTPTimeout` (30s)
  timeout that does not follow redirects: a 307/308 would re-POST the form, client secret included, to the redirect target. A redirect answer is
  reported as a failed request.
- **Injected client** — a `WithHTTPClient` client keeps its timeout and transport and is copied, never modified. Without a `CheckRedirect` the
  copy follows no redirect, like the built-in client. With one, a redirect to another origin (scheme, host, port) fails with
  `ErrRedirectNotAllowed` before the client's own policy is consulted; hops within the origin are left to that policy except for JWT bearer, which refuses every redirect. This holds with
  `WithInsecureHTTP()` too.
- **Construction** — every constructor (`ClientCredentials`, `Refresh`, `NewAuthCode`, `NewDeviceFlow`, `NewExchanger`, `NewRevoker`,
  `NewJWTBearer` and the `*FromDiscovery` variants) checks all its endpoint URLs with the https rule above and returns an error wrapping
  `ErrInsecureEndpoint` for one that is malformed or not https, before anything is sent. The `*FromDiscovery` constructors read the token
  endpoint from the discovery source once, at construction, and check it then. The transport check stays as a second line of defense.
  `ErrInsecureEndpoint` and `ErrRedirectNotAllowed` are never retried.
- **https only** — a request to an endpoint that is not https fails with `ErrInsecureEndpoint` before anything is sent. Plain http is accepted on a
  loopback host (`localhost` or a loopback IP literal); `WithInsecureHTTP()` accepts it on any host and is meant for a test IdP. The check runs on
  the transport, so it also covers the revocation and device endpoints and each redirect hop of an injected client.
  `ValidateEndpointURL(url, allowInsecureHTTP)` applies the same rule up front.
- **Logs and error text** — The identity provider is untrusted: its status line, headers, `Location`, body, and trailers may echo secrets. Returned
  error text and package logs withhold IdP bodies, error descriptions, error URIs, decoded values, and IdP-supplied URLs, including a followed redirect
  URL when a later hop fails. They may show the numeric HTTP status, registered RFC 6749/8628 error codes from the fixed allowlist, and body or
  withheld-field byte lengths.

  Errors raised before a response is parsed keep their text only for cancellation or deadline sentinels, all dial-phase `net.OpError` values (regardless
  of their inner cause), `net.DNSError`, the connection errnos `ECONNREFUSED`, `ECONNRESET`, `EHOSTUNREACH`, `ENETUNREACH`, and `ETIMEDOUT`, TLS
  certificate verification errors, `x509.CertificateInvalidError`, `x509.HostnameError`, `x509.UnknownAuthorityError`, `x509.SystemRootsError`,
  `tls.AlertError`, `tls.RecordHeaderError`, and package guard or grant sentinels. URL, syscall, DNS, and certificate-verification wrappers qualify only
  when every nested cause qualifies. Read/write and `proxyconnect` `net.OpError` values remain summarized; proxy setup can read peer bytes, including
  CONNECT responses. Any other pre-response failure, including an unparseable IdP response or an unknown wrapper, is summarized; its original cause
  remains reachable through `errors.As`. This rule also applies to every `RetryTransport` attempt when used on its own.

  Application-supplied HTTP clients/transports, `ClientAuthenticator` implementations, discovery sources, and configured endpoint URLs are trusted;
  errors they produce on their own are outside this guarantee. Original causes, including response bodies, remain available through
  `errors.Is`/`errors.As` where preserved; avoid logging those causes directly.

  Some `x/oauth2` body-read and JSON decoder errors are flattened upstream, so their original causes cannot be recovered through
  `errors.Is`/`errors.As`.

## Key types

| Type              | Description                                                                                           |
|-------------------|-------------------------------------------------------------------------------------------------------|
| `AuthCode`        | Authorization-code driver: `AuthCodeURL(state, ...)` then `Exchange(ctx, code, ...)`.                 |
| `Exchanger`       | RFC 8693 client: `Exchange(ctx, ExchangeRequest)` and self-refreshing `TokenSource(ctx, req)`.        |
| `ExchangeRequest` | One exchange: `SubjectToken` (required), optional actor token, `Audience`/`Resource`, `Scopes`.       |
| `JWTBearer`       | RFC 7523 §2.1 grant: `Token(ctx)` (context-bound, cached, serialized refresh) and `TokenSource(ctx)`. |

## Usage

Machine-to-machine, wired into an outbound gRPC client:

```go
src, err := oauth2client.ClientCredentials(ctx, tokenURL, clientID, clientSecret,
    oauth2client.WithScopes("orders:read", "orders:write"),
)
if err != nil {
    return err // tokenURL is malformed or not https
}
creds := client.NewInsecureTokenCredentials(src) // every call now carries a fresh token
```

Token exchange, propagating the caller's identity to a downstream service (RFC 8693):

```go
ex, err := oauth2client.NewExchanger(tokenURL, clientID, clientSecret)
if err != nil {
    return err
}
tok, err := ex.Exchange(ctx, oauth2client.ExchangeRequest{
    SubjectToken: inboundAccessToken,
    Audience:     "https://downstream.internal",
})
```

## JWT bearer grant (RFC 7523)

`NewJWTBearer(tokenURL, issuer, key, opts...)` acquires tokens with the JWT bearer authorization grant — the flow of Google service accounts. Each
exchange signs a fresh assertion with the asymmetric `jwt.SigningKey` (`iss` = issuer, `aud` = token URL, `scope` from `WithScopes`, `iat`, `exp` one
hour later; an empty `KeyID` omits the `kid` header) and posts `grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer` with the `assertion`. The
constructor signs a probe assertion, so an unusable key fails with `ErrJWTBearerConfig` up front.

The signed assertion is exchanged through `go-atlas-net/oauth2/jwtbearer.Exchange`. The response limit is 64 KiB and redirects are refused.

- **Context-bound** — `Token(ctx)` runs the exchange on `ctx`, so a deadline or cancellation bounds the token request. A cached token is served even
  on
  a done `ctx`.
- **Positive expiry required** — a response without a positive `expires_in` is rejected (`ErrTokenExchange`) instead of being cached forever.
- **Early refresh** — the token is reused until `WithEarlyExpiry` (default 10s) before its expiry.
- **No stampede** — refreshes are serialized per `JWTBearer`. Concurrent callers that find the cache stale wait for the one in-flight exchange and
  reuse
  its token; a waiter whose `ctx` ends gives up alone, and if the in-flight exchange fails (for example because its caller was canceled), the next
  waiter runs its own exchange on its own `ctx`. Failures are never cached.
- **Errors** — a signing or transport failure and a non-2xx response wrap `ErrTokenRequest`; a non-2xx response also carries an
  `*oauth2.RetrieveError` (status and RFC 6749 `error` code) reachable via `errors.As`. A malformed body wraps
  `ErrTokenExchange`.
  `WithRetryAttempts` retries transport errors, 429, and 5xx with a fresh assertion per attempt.

```go
bearer, err := oauth2client.NewJWTBearer(tokenURL, clientEmail,
    jwt.SigningKey{KeyID: keyID, Algorithm: jwt.AlgRS256, Key: rsaKey},
    oauth2client.WithScopes("https://www.googleapis.com/auth/cloud-platform"),
    oauth2client.WithEarlyExpiry(5*time.Minute),
    oauth2client.WithHTTPClient(client),
)
tok, err := bearer.Token(ctx) // bounded by ctx's deadline
```

Build one `JWTBearer` per credential and keep it; `TokenSource(ctx)` adapts it to `oauth2.TokenSource` with a pinned context.

## Discovery, Principal, and config

- **OIDC discovery** — `ClientCredentialsFromDiscovery` / `NewExchangerFromDiscovery` take the token endpoint from a `TokenEndpointSource` (a narrow
  `interface{ TokenEndpoint() string }` satisfied by [`oidc.Provider`](../oidc)) instead of a literal URL, so acquisition and verification point at
  the
  same IdP metadata. They return `ErrNoTokenEndpoint` if discovery has not resolved one and `ErrInsecureEndpoint` if the discovered endpoint is not
  https. The endpoint may be on another host than the issuer (some IdPs serve tokens from a dedicated domain); same-origin is the business of the
  discovery validation of `auth/oidc`.
- **Principal** — `Principal(subject, tok)` builds an [`auth/principal.Principal`](../principal) from a fetched token (subject + the granted `scope`),
  the same type the scope enforcer consumes. It does not verify the token.
- **Config + factory** — [`authconfig.OAuth2Client`](../../config/auth) + [`factory`](../../factory/auth/oauth2client) build a ready
  client_credentials
  `oauth2.TokenSource` (or `Exchanger`) from a YAML template, resolving the endpoint from `tokenURL` or OIDC `discoveryURL`.

## Client authentication (JWT assertion)

Enterprise / FAPI IdPs often require the client to authenticate with a signed JWT assertion (RFC 7523) instead of a shared secret. `WithClientAuth`
supplies one; it supersedes `WithAuthStyle`/`clientSecret` for `ClientCredentials` and `Exchanger` (the assertion is minted fresh per fetch —
`iss`=`sub`=client id, `aud`=token endpoint, unique `jti`, short `exp`). `Refresh` and `AuthCode` keep the secret path.

| Constructor                               | Scheme              | Signs the assertion with                         |
|-------------------------------------------|---------------------|--------------------------------------------------|
| `PrivateKeyJWT(clientID, jwt.SigningKey)` | `private_key_jwt`   | the client's asymmetric private key (kid → IdP). |
| `ClientSecretJWT(clientID, clientSecret)` | `client_secret_jwt` | the client secret via HS256 (secret never sent). |

```go
auth, _ := oauth2client.PrivateKeyJWT("svc", jwt.SigningKey{KeyID: "key-1", Algorithm: jwt.AlgRS256, Key: privKey})
src, err := oauth2client.ClientCredentials(ctx, tokenURL, "svc", "", oauth2client.WithClientAuth(auth))
```

Tune the assertion with `WithAssertionLifetime` and `WithAssertionAudience`. Via YAML, set the [`authconfig.OAuth2Client`](../../config/auth)
`clientAuth` block (`method` + PEM `privateKey`/`keyID`/`algorithm`) and the [factory](../../factory/auth/oauth2client) builds the authenticator for
you.

## Refresh timing and revocation

- **Early refresh** — `WithEarlyExpiry(d)` refreshes a token `d` before its `exp` to absorb clock skew and in-flight latency (for `ClientCredentials`
  and `Exchanger.TokenSource`). `Refresh`/`AuthCode` keep x/oauth2's default window — overriding it there would drop refresh-token rotation.
- **Revocation** — a `Revoker` built with `NewRevoker(revocationURL, id, secret)` and called as `Revoke(ctx, token, WithTokenTypeHint(...))`
  revokes an acquired token at the IdP (RFC 7009), reusing the same client auth (secret or assertion). Per the RFC the endpoint answers 200 even
  for an unknown token, so a nil error means "not (or no longer) valid".

**Retrying the native grants** — only `Exchanger` and `JWTBearer` retry on their own. To add retries to the x/oauth2-backed grants (
`ClientCredentials`,
`Refresh`, `AuthCode`, `DeviceFlow`), wrap the transport with `RetryTransport` and inject it via `WithHTTPClient`; it retries transport errors, 429,
and
5xx (only replayable requests; the last response is surfaced on exhaustion). Tune it with `WithTransportAttempts` (default
`DefaultTransportRetryAttempts`, 2 retries) and `WithTransportBackoff`:

```go
client := &http.Client{Transport: oauth2client.RetryTransport(http.DefaultTransport)}
src, err := oauth2client.ClientCredentials(ctx, tokenURL, id, secret, oauth2client.WithHTTPClient(client))
```

Two RFC edge cases are preserved verbatim (see `parseTokenResponse` godoc): a response without `expires_in` yields a never-expiring token (a
self-refreshing source won't refresh it), and `token_type: "N_A"` (RFC 8693) is not a bearer credential — don't attach it with `SetAuthHeader`. Client
secrets are held for the source lifetime and can't be zeroized; prefer `PrivateKeyJWT` when that matters.

## Observability

`WithMetrics(NewMetrics(collector, ""))` records, under subsystem `auth_oauth2client`:

| Metric                         | Type      | Labels            | Meaning                                              |
|--------------------------------|-----------|-------------------|------------------------------------------------------|
| `token_fetches_total`          | counter   | `grant`, `status` | Token acquisitions (cache hits are **not** counted). |
| `token_fetch_duration_seconds` | histogram | `grant`, `status` | Acquisition latency.                                 |
| `token_fetch_retries_total`    | counter   | `grant`           | Retries after a transient token-exchange failure.    |

`grant` is one of `client_credentials` / `refresh_token` / `authorization_code` / `token_exchange` / `device_code` / `jwt_bearer`. A metrics-wrapped
source records only genuine acquisitions — when the reuse cache serves an unexpired token, nothing is recorded. `WithLogger` logs fetch failures and
retry attempts at warn level (never the token itself). Both default to off, keeping the hot path allocation-free.

## Errors

`ClientCredentials`, `Refresh`, and `AuthCode` carry the underlying x/oauth2 error (typically an `*oauth2.RetrieveError` with the IdP status and
body), reachable via `errors.As`; the error text itself leaves the body out — except `ClientCredentials` with `WithClientAuth`, whose hand-rolled request wraps `ErrTokenRequest` for a transport failure or
a
non-2xx response; an unparsable body or one without `access_token` wraps `ErrTokenExchange` (shared response parser). `Exchanger` returns
`ErrSubjectTokenRequired` for a missing subject token and wraps `ErrTokenExchange` for a transport failure, a non-2xx response, or an unparsable
body —
match both with `errors.Is`. `Revoker.Revoke` wraps `ErrRevocation`, `DeviceFlow` wraps `ErrDeviceAuth`, a client authenticator that cannot build or
sign its assertion wraps `ErrClientAssertion`, `NewJWTBearer` returns `ErrJWTBearerConfig` for an unusable configuration, and the discovery-based
constructors return `ErrNoTokenEndpoint` when the endpoint has not resolved.

## See also

- [`auth/oidc`](../oidc) — verify inbound tokens; consumes the same `oauth2.TokenSource` for userinfo.
- [`auth/selfjwt`](../selfjwt) — mint self-issued tokens.
- [`transport/grpc/client`](../../transport/grpc/client) — `NewInsecureTokenCredentials` attaches a `TokenSource` to gRPC calls.
