# vault

```go
import "github.com/altessa-s/go-atlas/security/vault"
```

Package `vault` provides a high-level HashiCorp Vault client with pluggable authentication and automatic token renewal. Wraps the official Vault API
client with support for AppRole, Token, and UserPass auth.

## Functions

| Function / Method       | Description                               |
|-------------------------|-------------------------------------------|
| `New`                   | Create Vault client (auth method via `WithAuthMethod`) |
| `RunRenewalWithContext` | Start background token renewal; blocks until the first token is obtained |
| `StopRenewal`           | Stop renewal goroutine and wait for it    |
| `Close`                 | Stop renewal, shut the auth method down and revoke a token it minted |
| `RawClient`             | Access underlying `*api.Client`           |
| `RenewalErr`            | Error that ended the current renewal run unexpectedly, or `nil` |
| `CheckHealth`           | Health check (`health.Checker`); not serving without working authentication |

## Renewal lifecycle

`RunRenewalWithContext` returns `nil` only once a token has actually been obtained; a run that ends first returns its authentication error,
`ErrTimeout`, the context error, or `ErrRenewalStopped`. Each call stops the previous run and starts a fresh one only after the previous run has exited,
so runs never overlap. The auth method keeps its credentials when a run ends, so renewal can be started again after a run failed, timed out or was
stopped; only `Close` shuts the method down, which zeroes the credentials of the token, AppRole, and UserPass methods. `StopRenewal` waits for the run
to exit, which is bounded as long as the auth method honors context cancellation.

A failed login that may succeed later is retried with exponential backoff (2s doubling to 30s, with jitter), which a successful login resets. A
run that ends on its own with an error, such as a re-login rejected for an expired AppRole secret ID, is logged at error level and recorded:
`RenewalErr` returns the error and `CheckHealth` reports not serving until a later `RunRenewalWithContext` logs in.

`CheckHealth` consults the unauthenticated `sys/health` probe only for a client with working authentication. With an auth method that means
renewal was started and not stopped, its run logged in, and it has not ended with an error; a run that ended because the token never expires
still counts. Without an auth method it means the client carries a token the caller set. Before the first login completes, after a failed start,
after `StopRenewal` and after `Close` the client is not serving.

The context passed to `RunRenewalWithContext` bounds the whole renewal run, not only the first login. When it ends after the login, without
`StopRenewal` or `Close`, nothing renews the token any more: `RenewalErr` returns `ErrRenewalCanceled` and `CheckHealth` reports not serving
until renewal is started again. A run whose token never expires has already ended by then and stays healthy. After an explicit `StopRenewal`
`RenewalErr` stays `nil` and `CheckHealth` reports not serving, whatever the lease.

A token supplied through the token auth method cannot be replaced by logging in again. When it is non-renewable with a finite TTL (a batch
token), the run does not poll Vault: it logs the coming expiry once as a warning and, when the TTL has passed, ends with `auth.ErrTokenExpired`,
which `RenewalErr` and `CheckHealth` then report.

A non-renewable token with a finite TTL keeps the run alive: the authenticator logs in again after two thirds of the remaining lease,
independently of the failure backoff, so a short TTL does not lapse while Vault is healthy. A run ends on its own only when the token never expires
or authentication fails permanently; the latter is logged at error level.

## Shutdown

`Close` marks the client closed, calls `StopRenewal`, shuts the auth method down and revokes the client token (`auth/token/revoke-self`) when a renewal run obtained it from an auth method that
implements `auth.TokenIssuer` (AppRole, UserPass). The login is recorded on the run's own goroutine, so a token issued while the start was timing
out is revoked as well. After `Close`, `RunRenewalWithContext` returns `ErrClosed` and `CheckHealth` reports not serving. A token supplied by the caller, through the token auth method or on a client passed to `New`, is
never revoked. Revocation is best effort: it is bounded by `DefaultRevokeTimeout` (5s) and a failure is logged, not returned.

## TLS

`WithTLSConfig` applies to the client `New` creates. `New` returns `ErrTLSConfigNotApplied` when the configuration cannot be installed: the HTTP
transport is not an `*http.Transport`, or `WithVaultClient` supplied a ready-made client, which carries its own transport.

## Subpackages

| Package                                 | Description                                  |
|-----------------------------------------|----------------------------------------------|
| [auth](./auth)                          | Authentication methods and lifecycle manager |
| [factory](../../factory/security/vault) | Config-based Vault client creation           |
