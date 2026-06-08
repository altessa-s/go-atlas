# adapters

```go
import "github.com/altessa-s/go-atlas/observability/metrics/adapters"
```

Package `adapters` defines the `Adapter` interface for metrics backends. Implement this interface to translate abstract metric operations
into a specific backend format (Prometheus, StatsD, OpenTelemetry, etc.). Use `MultiAdapter` to fan out to several.

## Key types

| Type           | Description                                                                                |
|----------------|--------------------------------------------------------------------------------------------|
| `Adapter`      | Interface: `Register`, `RecordCounter`, `RecordGauge`, `RecordHistogram`, `Flush`, `Close` |
| `MultiAdapter` | Broadcasts operations to multiple adapters simultaneously                                  |
| `Desc`         | Metric descriptor passed during registration                                               |

## Subpackages

| Package                    | Description                              |
|----------------------------|------------------------------------------|
| [memory](./memory)         | In-memory adapter for tests and assertions |
| [prometheus](./prometheus) | Prometheus adapter using `client_golang` |
