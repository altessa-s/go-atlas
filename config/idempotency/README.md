# idempotencyconfig

```go
import idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
```

Package `idempotencyconfig` defines the idempotency key storage schema. Schemas are populated by [`config/loader`](../loader) and consumed by the
component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type          | Description                                               |
|---------------|-----------------------------------------------------------|
| `Config` | Defines the configuration for idempotency key management. |

See the [config index](../README.md) for the other schema packages.
