# auth

```go
import "github.com/altessa-s/go-atlas/security/vault/auth"
```

Package `auth` provides pluggable authentication methods for HashiCorp Vault with automatic token renewal and lifecycle management.

## Key types

| Type / Interface  | Description                                       |
|-------------------|---------------------------------------------------|
| `Method`          | Interface: `Authenticate()`, `Shutdown()`, `Name()` |
| `Authenticator`   | Manages auth lifecycle with background renewal    |
| `TokenIssuer`     | Optional `Method` interface: `IssuesToken()` marks tokens the Vault client may revoke on close |

A `Method` whose `Authenticate` succeeds without a secret or without `Auth` breaks the contract: `Authenticator.Run` reports
`ErrEmptyResponse` on `ErrorsCh` and stops, as for other permanent errors.

`Authenticator.Run` returns `nil` when its context is canceled or the token never expires (lease duration 0). Any other exit returns the error
that ended the run and logs it at error level. A non-renewable token with a finite TTL does not end the run when the method mints its tokens
(`TokenIssuer`); for a method that installs a caller-supplied token `Run` logs the coming expiry once, does not log in again, and returns
`ErrTokenExpired` when the TTL has passed. For an issuing method `Run` logs in again after two thirds
of the remaining lease and no later than 250ms before expiry, so the method's credentials stay available until the run ends. This refresh schedule
is separate from the failure backoff, which applies only after a failed login. A lease too short to refresh ahead of expiry is logged once as a
warning and refreshed every 100ms.

A failed login that may succeed later is retried after a backoff that starts at `WithBackoffBase` (2s), doubles with every consecutive failure up
to `WithBackoffMax` (30s) and carries jitter. A login rejected for its credentials (`IsAuthenticationError`) ends the run.

The backoff is reset by a login whose token was put to use: a non-renewable token at once, a renewable one when its renewal cycle held it for at
least half of the lease granted at login, renewals included. A renewable token whose cycle ends earlier (renewal failing right after login, or a
lease that cannot be extended) counts as one more failure, so such logins are spaced by the same growing backoff instead of a tight loop. That
wait never runs past the lease of the token still held, less the 250ms safety margin, with the 100ms minimum as a floor.

`Run` shuts the method down when it returns, which zeroes its credentials. `WithKeepMethodOnExit` leaves the method usable for a later run; the
owner then calls `Shutdown` once the method is retired (the Vault client does so in `Close`). A method asked to authenticate after `Shutdown`
returns `ErrMethodShutDown`, which is not retried. `WithOnLogin` registers a callback that `Run` calls on its goroutine after every successful
login.

## Subpackages

| Package                  | Description                            |
|--------------------------|----------------------------------------|
| [approle](./approle)    | AppRole auth (role ID + secret ID)     |
| [token](./token)        | Direct token-based auth                |
| [userpass](./userpass)   | Username/password auth                 |
