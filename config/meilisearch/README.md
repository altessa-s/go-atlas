# meilisearchconfig

```go
import meilisearchconfig "github.com/altessa-s/go-atlas/config/meilisearch"
```

Package `meilisearchconfig` defines the Meilisearch connection schema. Schemas are populated by [`config/loader`](../loader) and consumed by the
component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type          | Description                                                            |
|---------------|------------------------------------------------------------------------|
| `Config` | Represents the configuration for connecting to a Meilisearch instance. |

See the [config index](../README.md) for the other schema packages.
