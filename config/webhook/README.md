# webhookconfig

```go
import webhookconfig "github.com/altessa-s/go-atlas/config/webhook"
```

Package `webhookconfig` defines the webhook signing schema. Schemas are populated by [`config/loader`](../loader) and consumed by the component
factories, which map them to generated options; runtime packages never import them.

## Key types

| Type               | Description                                                                             |
|--------------------|-----------------------------------------------------------------------------------------|
| `Signature` | Configures HMAC signing and verification of webhook request bodies (security/hmacsign). |

See the [config index](../README.md) for the other schema packages.
