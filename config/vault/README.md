# vaultconfig

```go
import vaultconfig "github.com/altessa-s/go-atlas/config/vault"
```

Package `vaultconfig` defines the HashiCorp Vault client schema. Schemas are populated by [`config/loader`](../loader) and consumed by the component
factories, which map them to generated options; runtime packages never import them.

## Key types

| Type                | Description                                                              |
|---------------------|--------------------------------------------------------------------------|
| `AuthMethod`   | Represents the authentication method used to authenticate with Vault.    |
| `AuthAppRole`  | Represents the AppRole authentication method.                            |
| `AuthUserPass` | Represents the Userpass authentication method.                           |
| `Auth`         | Configures authentication settings for connecting to Vault.              |
| `Config`             | Represents the configuration for connecting to a HashiCorp Vault server. |

See the [config index](../README.md) for the other schema packages.
