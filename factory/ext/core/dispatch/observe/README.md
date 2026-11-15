# observe

```go
import "github.com/altessa-s/go-atlas/factory/ext/core/dispatch/observe"
```

Metrics adapter for the go-atlas-core [`runtime/dispatch`](https://github.com/altessa-s/go-atlas-core/tree/develop/runtime/dispatch) engine: an
`Observer` that turns engine events into `metrics.Meter` instruments, keeping the engine free of the observability stack.

## Options

| Option                 | Default   | Purpose                                 |
|------------------------|-----------|-----------------------------------------|
| `WithMetricsSubsystem` | `"async"` | Subsystem the metrics are registered in |

## Metrics

| Metric                         | Type      | Meaning                                              |
|--------------------------------|-----------|------------------------------------------------------|
| `items_enqueued_total`         | counter   | Items admitted to the buffer or the WAL              |
| `items_dropped_total`          | counter   | Items rejected on admission                          |
| `batch_flush_duration_seconds` | histogram | Time a batch spent in the sink, retries included     |
| `sink_errors_total`            | counter   | Items of failed sink attempts, retries included      |
| `wal_errors_total`             | counter   | WAL append failures                                  |
| `encode_errors_total`          | counter   | Codec encode failures                                |
| `workers_active`               | gauge     | Running dispatch workers                             |
| `wal_bytes`                    | gauge     | Bytes held by the WAL across all segments            |
| `wal_replay_total`             | counter   | Records replayed from the WAL on startup             |

## Usage

```go
eng, err := dispatch.NewEngine[Event](sink,
    dispatch.WithObserver[Event](observe.New(meter, observe.WithMetricsSubsystem("audit"))),
)
```

The [dispatch factory](../README.md) installs it when given a meter (`EngineBuilder.WithMeter`).
