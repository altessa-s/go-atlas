# vault

```go
import "github.com/altessa-s/go-atlas/security/vault"
```

Package `vault` provides a high-level HashiCorp Vault client with pluggable authentication and automatic token renewal. Wraps the official
Vault API client with support for AppRole, Token, and UserPass auth.

## Functions

| Function / Method       | Description                               |
|-------------------------|-------------------------------------------|
| `New`                   | Create Vault client with auth method      |
| `DefaultConfig`         | Default Vault configuration               |
| `RunRenewalWithContext` | Start background token renewal            |
| `StopRenewal`           | Stop renewal goroutine and wait for it    |
| `Client`                | Access underlying `*api.Client`           |
| `CheckConnection`       | Health check                              |
| `WaitFirstRenew`        | Block until first token is obtained       |

## Renewal lifecycle

`RunRenewalWithContext` returns `nil` only once a token has actually been obtained; a run that ends first returns its authentication
error, `ErrTimeout`, the context error, or `ErrRenewalStopped`. Each call stops the previous run and starts a fresh one only after the
previous run has exited, so runs never overlap. A run shuts its auth method down when it ends, and the token, AppRole, and UserPass
methods zero their credentials on shutdown, so restarting renewal after a run ended needs a method that can authenticate again.
`StopRenewal` waits for the run to exit, which is bounded as long as the auth method honors context cancellation.

## Subpackages

| Package              | Description                                   |
|----------------------|-----------------------------------------------|
| [auth](./auth)       | Authentication methods and lifecycle manager  |
| [factory](./factory) | Config-based Vault client creation            |
