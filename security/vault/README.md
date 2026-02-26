# vault

```go
import "github.com/altessa-s/go-atlas/security/vault"
```

Package `vault` provides a high-level HashiCorp Vault client with pluggable authentication and automatic
token renewal. Wraps the official Vault API client with support for AppRole, Token, and UserPass auth.

## Usage

```go
v, err := vault.New(ctx,
    vault.WithAddress("https://vault.example.com:8200"),
    vault.WithAuthMethod(approle.New(roleID, secretID)),
)
if err != nil {
    return err
}

v.RunRenewal()
defer v.StopRenewal()

client := v.Client() // *api.Client
```

## Functions

| Function / Method      | Description                               |
|------------------------|-------------------------------------------|
| `New`                  | Create Vault client with auth method      |
| `DefaultConfig`        | Default Vault configuration               |
| `RunRenewal`           | Start background token renewal            |
| `StopRenewal`          | Stop renewal goroutine                    |
| `Client`               | Access underlying `*api.Client`           |
| `CheckConnection`      | Health check                              |
| `WaitFirstRenew`       | Block until first token is obtained       |

## Subpackages

| Package              | Description                                   |
|----------------------|-----------------------------------------------|
| [auth](./auth)       | Authentication methods and lifecycle manager  |
| [factory](./factory) | Config-based Vault client creation            |
