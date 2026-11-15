# aipmeter

```go
import "github.com/altessa-s/go-atlas/observability/metrics/aipmeter"
```

Adapts a go-atlas [`metrics.Meter`](../README.md) to the metrics API of [go-aip](https://github.com/altessa-s/go-aip/tree/develop/metrics), so the
go-aip filter, order and projection parsers and the backend translators record into the application's collector. go-aip declares its own narrow
`Meter` (counters and timers only) to depend on no metrics backend; `aipmeter.New` forwards it to the go-atlas one.

```go
collector := metrics.New(metrics.WithServiceName("app"), metrics.WithAdapter(prometheusAdapter))

p, err := parser.New(parser.WithCollector(aipmeter.New(collector)))           // go-aip filter/parser
policy, err := orderby.NewPolicy(...)
tr, err := mongo.NewOrderTranslator(policy, mongo.WithOrderCollector(aipmeter.New(collector))) // go-aip backend/mongo
```

| Recorded by                                             | Metric                                                                                   |
|---------------------------------------------------------|------------------------------------------------------------------------------------------|
| go-aip `filter/parser`, `orderby`, `projection`         | `{service}_{filter,orderby,projection}_parse_duration_seconds`, `..._parse_errors_total` |
| go-aip backend filter, order and projection translators | `{service}_{filter,orderby,projection}_<backend>_translate_duration_seconds`             |

`New(nil)` (a typed nil included) returns go-aip `metrics.Noop()`. The counters and timers are the go-atlas ones, created on the wrapped meter;
timers use the collector's default buckets. See the [metrics reference](../../../docs/metrics.md).
