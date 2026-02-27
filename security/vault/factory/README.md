# factory

```go
import "github.com/altessa-s/go-atlas/security/vault/factory"
```

Package `factory` provides configuration-based creation of Vault clients. Reads from `config.Vault` to select the authentication method
(AppRole, Token, UserPass) and connection settings.
