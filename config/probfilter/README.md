# probfilterconfig

```go
import probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
```

Package `probfilterconfig` defines the probabilistic filter (Bloom, Cuckoo) schemas. Schemas are populated by [`config/loader`](../loader) and consumed
by the component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type                                | Description                                            |
|-------------------------------------|--------------------------------------------------------|
| `Type`           | Defines the type of probabilistic filter to use.       |
| `StorageType`    | Defines the storage backend for probabilistic filters. |
| `BloomDefaults`  | Defines default settings for Bloom filters.            |
| `CuckooDefaults` | Defines default settings for Cuckoo filters.           |
| `Defaults`       | Defines default settings for all filters.              |
| `BloomConfig`    | Defines configuration for a specific Bloom filter.     |
| `CuckooConfig`   | Defines configuration for a specific Cuckoo filter.    |
| `Filter`         | Defines configuration for a single named filter.       |
| `Config`               | Defines the configuration for probabilistic filters.   |

See the [config index](../README.md) for the other schema packages.
