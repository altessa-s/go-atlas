# vault

```go
import "github.com/altessa-s/go-atlas/security/tlsutils/providers/vault"
```

Package `vault` provides a HashiCorp Vault PKI engine certificate provider. Generates certificates dynamically
and supports automatic renewal.

## Usage

```go
p := vault.New(
    vault.WithClient(vaultClient),
    vault.WithPKIPath("pki/issue/my-role"),
    vault.WithCommonName("myapp.internal"),
    vault.WithTTL("24h"),
)
defer p.Close(ctx)

tlsCfg := p.TLSConfig()
```
