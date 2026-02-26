# userpass

```go
import "github.com/altessa-s/go-atlas/security/vault/auth/userpass"
```

Package `userpass` provides username/password authentication for HashiCorp Vault. Suitable for
interactive applications and development environments.

## Usage

```go
method := userpass.New(username, password,
    userpass.WithMountPath("auth/userpass"),
)
```
