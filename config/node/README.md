# nodeconfig

```go
import nodeconfig "github.com/altessa-s/go-atlas/config/node"
```

Package `nodeconfig` defines the service node identity schema. Schemas are populated by [`config/loader`](../loader) and consumed by the component
factories, which map them to generated options; runtime packages never import them.

## Key types

| Type   | Description                                      |
|--------|--------------------------------------------------|
| `Config` | Represents the configuration for a service node. |

See the [config index](../README.md) for the other schema packages.
