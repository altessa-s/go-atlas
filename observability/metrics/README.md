# metrics

```go
import "github.com/altessa-s/go-atlas/observability/metrics"
```

Package `metrics` provides an abstract metrics collection system. Components depend on abstract interfaces (`Counter`, `Gauge`,
`Histogram`, `Timer`); export happens through pluggable adapters such as Prometheus, StatsD, or OpenTelemetry.

## Key types

| Type / Function | Description                                                 |
|-----------------|-------------------------------------------------------------|
| `Collector`     | Create and manage metrics; supports `WithSubsystem` scoping |
| `Counter`       | Monotonically increasing value                              |
| `Gauge`         | Value that can increase and decrease                        |
| `Histogram`     | Distribution of observed values                             |
| `Timer`         | Duration measurement with `Start()` / `ObserveDuration()`   |
| `Labels`        | `map[string]string` for metric dimensions                   |
| `Noop()`        | Zero-cost no-op implementation                              |

## Naming convention

Full metric name: `{namespace}_{subsystem}_{name}` (e.g., `myapp_broker_requests_total`).
Subsystem is optional and defaults to empty.

## Interner metrics

`InternerMetrics` bridges the core string interner's performance counters to the metrics system.
Call `Report()` periodically (e.g., via `service/scheduler`) to push hit rate, miss, and eviction data.

```go
im := metrics.NewInternerMetrics(collector, strings.GlobalInterner())
sched.Register(ctx, scheduler.TaskConfig{
    ID:       "interner-metrics",
    Schedule: "0 */1 * * * *",
    Func:     func(ctx context.Context) error { im.Report(); return nil },
})
```

Exported metrics (subsystem `interner`):

| Metric                    | Type    | Description                              |
|---------------------------|---------|------------------------------------------|
| `interner_hot_hits_total`  | Counter | Lookups served from hot cache            |
| `interner_cold_hits_total` | Counter | Lookups served from cold cache           |
| `interner_misses_total`    | Counter | Lookups that created a new entry         |
| `interner_evictions_total` | Counter | Entries removed by LRU eviction          |
| `interner_current_size`    | Gauge   | Current number of interned strings       |
| `interner_hit_rate`        | Gauge   | Overall hit rate (hot + cold) / total    |
| `interner_hot_hit_rate`    | Gauge   | Hot cache hit rate                       |

## Regex cache metrics

`RegexCacheMetrics` bridges the CEL filter regex pattern cache to the metrics system.
Uses callback functions to avoid import cycles (`data/filter` → `observability/metrics`).

```go
rcm := metrics.NewRegexCacheMetrics(collector,
    filter.RegexCacheStatsSnapshot,
    filter.ResetRegexCacheStats,
)
sched.Register(ctx, scheduler.TaskConfig{
    ID:       "regex-cache-metrics",
    Schedule: "0 */1 * * * *",
    Func:     func(ctx context.Context) error { rcm.Report(); return nil },
})
```

Exported metrics (subsystem `regex_cache`):

| Metric                    | Type    | Description                              |
|---------------------------|---------|------------------------------------------|
| `regex_cache_hits_total`   | Counter | Pattern lookups served from cache        |
| `regex_cache_misses_total` | Counter | Lookups requiring regex compilation      |
| `regex_cache_current_size` | Gauge   | Current number of compiled patterns      |
| `regex_cache_hit_rate`     | Gauge   | Cache hit rate (hits / total lookups)    |

## Subpackages

| Package                                      | Description                                      |
|----------------------------------------------|--------------------------------------------------|
| [adapters](./adapters)                       | Adapter interface and `MultiAdapter` broadcaster |
| [adapters/memory](./adapters/memory)         | In-memory adapter for tests and assertions       |
| [adapters/prometheus](./adapters/prometheus)  | Prometheus backend with HTTP `/metrics` handler  |
| [factory](./factory)                         | Configuration-based `Collector` creation         |
