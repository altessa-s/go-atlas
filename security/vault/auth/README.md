# auth

```go
import "github.com/altessa-s/go-atlas/security/vault/auth"
```

Package `auth` provides pluggable authentication methods for HashiCorp Vault with automatic token
renewal and lifecycle management.

## Key types

| Type / Interface  | Description                                       |
|-------------------|---------------------------------------------------|
| `Method`          | Interface: `Authenticate()`, `Shutdown()`, `Name()` |
| `Authenticator`   | Manages auth lifecycle with background renewal    |

## Usage

```go
method := approle.New(roleID, secretID)
authenticator := auth.NewAuthenticator(client, method,
    auth.WithLogger(logger),
)

go authenticator.Run(ctx)
<-authenticator.FirstRenewCh() // wait for first token
defer authenticator.Stop()
```

## Subpackages

| Package                  | Description                            |
|--------------------------|----------------------------------------|
| [approle](./approle)    | AppRole auth (role ID + secret ID)     |
| [token](./token)        | Direct token-based auth                |
| [userpass](./userpass)   | Username/password auth                 |
