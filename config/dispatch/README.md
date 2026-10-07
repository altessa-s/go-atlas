# dispatchconfig

```go
import dispatchconfig "github.com/altessa-s/go-atlas/config/dispatch"
```

Package `dispatchconfig` defines the async dispatch engine schema and its write-ahead log. Schemas are populated by [`config/loader`](../loader) and
consumed by the component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type       | Description                                                    |
|------------|----------------------------------------------------------------|
| `Config` | Defines the configuration for an async dispatch engine.        |
| `WAL`      | Configures a write-ahead log backing an async dispatch engine. |

See the [config index](../README.md) for the other schema packages.
