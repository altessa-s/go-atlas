# metrics

```go
import "github.com/altessa-s/go-atlas/observability/metrics"
```

Package `metrics` provides an abstract metrics collection system. Components depend on abstract interfaces (`Counter`, `Gauge`, `Histogram`, `Timer`);
export happens through pluggable adapters (Prometheus and in-memory ship in [adapters](./adapters); other backends implement `adapters.Adapter`).

## Key types

| Type / Function                                                          | Description                                                                                    |
|--------------------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| `New`                                                                    | Create a `Collector` (`WithServiceName` default `appinfo.Name`, `WithAdapter`, `WithLogger`)   |
| `Meter`                                                                  | Create metrics (`Counter`, `Gauge`, `Histogram`, `Timer`); `WithSubsystem` returns a `Meter`   |
| `RegisterCounter`, `RegisterGauge`, `RegisterHistogram`, `RegisterTimer` | Checked registration returning `(T, error)` on invalid options, descriptor conflict, or error  |
| `MustCounter`, `MustGauge`, `MustHistogram`, `MustTimer`, `MustMeter`    | Checked registration helpers that panic on invalid options, descriptor conflict, or error      |
| `Collector`                                                              | `Meter` plus lifecycle (`Shutdown`, `ForceFlush`); held by the composition root                |
| `Counter`                                                                | Monotonically increasing value                                                                 |
| `Gauge`                                                                  | Value that can increase and decrease                                                           |
| `Histogram`                                                              | Distribution of observed values                                                                |
| `Timer`                                                                  | Duration measurement with `Start()` / `ObserveDuration()`                                      |
| `Labels`                                                                 | `map[string]string` for metric dimensions (cloned by `WithLabels`)                             |
| `ErrDescriptorConflict`                                                  | Returned when a metric name is redeclared with another kind, label set, help, or bucket list   |
| `Noop()`                                                                 | Zero-cost no-op implementation                                                                 |
| `IsNoop`                                                                 | Report whether a `Meter` is the `Noop()` singleton                                             |

## Meter vs Collector

Libraries accept a `Meter` (through `WithMeter` / `WithMetrics` options) and never see the lifecycle, so no component can shut down or flush the shared
collector. The application builds the `Collector`, passes it (or a `WithSubsystem` scope) to components, and calls `Shutdown` on exit. After `Shutdown`
is called, existing instruments and subsequent registrations become no-ops, and repeated `Shutdown` calls return the completed shutdown result.

Each metric name owns one canonical descriptor (`kind`, sorted `LabelNames`, `Help`, `Buckets`). Compatible redeclarations return the same instrument.
Use `RegisterCounter` / `RegisterGauge` / `RegisterHistogram` / `RegisterTimer` for checked error returns, or `MustCounter` / `MustGauge` /
`MustHistogram` / `MustTimer` (and `MustMeter`) to panic on failure. Calling the unchecked `Meter` methods with an incompatible descriptor or a failing
backend logs once at `Error` and returns a no-op handle.

## Naming convention

Full metric name: `{namespace}_{subsystem}_{name}` (e.g., `myapp_broker_requests_total`), where the namespace is the `WithServiceName` value. Subsystem
is optional and defaults to empty.

## Interner metrics

`InternerMetrics` bridges the core string interner's cumulative performance counters to the metrics system without resetting the interner. Call
`Report()` periodically (e.g., via the [go-horai](https://github.com/altessa-s/go-horai) scheduler) to push hit rate, miss, and eviction deltas;
concurrent calls are serialized safely.

```go
im := metrics.NewInternerMetrics(collector, intern.Default())
sched.Register(ctx, schedtask.Config{
    ID:       "interner-metrics",
    Schedule: "0 */1 * * * *",
    Func:     func(ctx context.Context) error { im.Report(); return nil },
})
```

Exported metrics (subsystem `interner`):

| Metric                     | Type    | Description                           |
|----------------------------|---------|---------------------------------------|
| `interner_hot_hits_total`  | Counter | Lookups served from hot cache         |
| `interner_cold_hits_total` | Counter | Lookups served from cold cache        |
| `interner_misses_total`    | Counter | Lookups that created a new entry      |
| `interner_evictions_total` | Counter | Entries removed by LRU eviction       |
| `interner_current_size`    | Gauge   | Current number of interned strings    |
| `interner_hit_rate`        | Gauge   | Overall hit rate (hot + cold) / total |
| `interner_hot_hit_rate`    | Gauge   | Hot cache hit rate                    |

## go-aip parsers and translators

The go-aip filter, order and projection parsers and translators record through go-aip's own narrow `Meter`;
[`aipmeter.New`](aipmeter/README.md) adapts a go-atlas `Meter` to it.

## Regex cache metrics

`RegexCacheMetrics` bridges the regex pattern cache of a go-aip `filter.Policy` (`Policy.RegexCacheStats`) to the metrics system through a callback, so
this package does not import go-aip. `Report()` is safe for concurrent calls and preserves post-reset counts (`cur < prev` adds `cur`).

```go
rcm := metrics.NewRegexCacheMetrics(collector,
    func() metrics.RegexCacheStatsSnapshot { // stats provider (cumulative)
        hits, misses, size := policy.RegexCacheStats() // policy is a go-aip *filter.Policy
        return metrics.RegexCacheStatsSnapshot{Hits: hits, Misses: misses, Size: int64(size)}
    },
)
sched.Register(ctx, schedtask.Config{
    ID:       "regex-cache-metrics",
    Schedule: "0 */1 * * * *",
    Func:     func(ctx context.Context) error { rcm.Report(); return nil },
})
```

Exported metrics (subsystem `regex_cache`):

| Metric                     | Type    | Description                           |
|----------------------------|---------|---------------------------------------|
| `regex_cache_hits_total`   | Counter | Pattern lookups served from cache     |
| `regex_cache_misses_total` | Counter | Lookups requiring regex compilation   |
| `regex_cache_current_size` | Gauge   | Current number of compiled patterns   |
| `regex_cache_hit_rate`     | Gauge   | Cache hit rate (hits / total lookups) |

## Subpackages

| Package                                        | Description                                      |
|------------------------------------------------|--------------------------------------------------|
| [adapters](./adapters)                         | Adapter interface and `MultiAdapter` broadcaster |
| [adapters/memory](./adapters/memory)           | In-memory adapter for tests and assertions       |
| [adapters/prometheus](./adapters/prometheus)   | Prometheus backend with HTTP `/metrics` handler  |
| [factory](../../factory/observability/metrics) | Configuration-based `Collector` creation         |
