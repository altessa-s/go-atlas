# secretsconfig

```go
import secretsconfig "github.com/altessa-s/go-atlas/config/secrets"
```

Package `secretsconfig` defines the secret manager schema. Schemas are populated by [`config/loader`](../loader) and consumed by the component
factories, which map them to generated options; runtime packages never import them.

## Key types

| Type              | Description                                             |
|-------------------|---------------------------------------------------------|
| `Provider` | Defines the type of secrets storage provider.           |
| `Cache`    | Configures the secrets cache behavior.                  |
| `Vault`    | Contains Vault-specific secrets provider configuration. |
| `GCP`      | Contains GCP Secret Manager-specific configuration.     |
| `Lockbox`  | Contains Yandex Cloud Lockbox-specific configuration.   |
| `Config`         | Defines the configuration for secrets management.       |

See the [config index](../README.md) for the other schema packages.
