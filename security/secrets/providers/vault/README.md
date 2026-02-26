# vault

```go
import "github.com/altessa-s/go-atlas/security/secrets/providers/vault"
```

Package `vault` provides a HashiCorp Vault KV v2 engine provider for the Atlas secrets system. Supports
versioning, metadata management, concurrent operations, and flexible path configuration.

## Usage

```go
store := vault.New[MySecret](client,
    vault.WithMountPath("secret"),
    vault.WithBasePath("myapp/prod"),
    vault.WithKeyDecoder(base64Dec),
    vault.WithValueDecoder(jsonDec),
)

mgr := secrets.New[MySecret](store)
```
