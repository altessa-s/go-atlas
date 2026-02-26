# factory

```go
import "github.com/altessa-s/go-atlas/security/vault/factory"
```

Package `factory` provides configuration-based creation of Vault clients. Reads from `config.Vault` to
select the authentication method (AppRole, Token, UserPass) and connection settings.

## Usage

```go
f := factory.New(factory.WithLogger(logger))
v, err := f.CreateVaultFromConfig(ctx, &cfg.Vault)
if err != nil {
    return err
}
defer v.StopRenewal()
```
