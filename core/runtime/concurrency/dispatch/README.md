# dispatch

```go
import "github.com/altessa-s/go-atlas/core/runtime/concurrency/dispatch"
```

Package `dispatch` provides a generic, non-blocking, batching dispatch
engine with optional crash-safe persistence backed by
[`core/io/wal`](../../../io/wal).

It is the foundation for fire-and-forget producers (audit logs, request
access logs, telemetry) where the hot path must not block on I/O but
events must still survive process crashes.

## Hot path

Without a WAL, `Engine.Submit` only sends the item to an in-memory channel
and returns. With `WithWAL` enabled, `Submit` additionally serializes the
item via the configured `Codec`, appends it to the active WAL segment
(page-cache write, no fsync) and then sends it to the channel. A
background goroutine fsyncs the active segment every fsync interval
(group commit), bounding the loss window after a crash.

Worker goroutines drain the channel, build batches up to `BatchSize` or
`FlushInterval`, and call `Sink.StoreBatch`. On success the corresponding
WAL records are acked; once all records in a sealed segment are acked,
the segment file is removed. Sinks receive **at-least-once** delivery: a
single item may reappear after a crash-and-replay sequence, so
implementations should be idempotent on the records' natural identity
(e.g. event ID).

## Durability guarantees

| Scenario                      | Guarantee                                                    |
|-------------------------------|--------------------------------------------------------------|
| Graceful `Shutdown(ctx)`      | Full drain to `Sink`, acked segments removed                 |
| `SIGKILL` / `panic` / OOM     | Records past the last fsync are replayed on next `Start`    |
| Loss window                   | Bounded by the WAL fsync interval (default 5 ms)             |
| Sink temporarily unavailable  | Records accumulate in the WAL up to its configured max bytes |
| WAL disabled                  | Zero I/O on the submit path; loses buffered items on crash   |

## Engine

| Function / Method    | Description                                                |
|----------------------|------------------------------------------------------------|
| `NewEngine[T]`       | Construct an `Engine` for item type `T` with a `Sink[T]`   |
| `Engine.Start`       | Open the WAL, replay unflushed records, start workers      |
| `Engine.Submit`      | Non-blocking enqueue (returns `false` on full-buffer drop) |
| `Engine.Shutdown`    | Drain pending items and close the WAL, bounded by `ctx`    |
| `Engine.Dropped`     | Total items dropped at submit time                         |
| `Engine.WAL`         | Underlying `*wal.WAL`, or nil when WAL is disabled         |

## Options

| Option                  | Effect                                                     |
|-------------------------|------------------------------------------------------------|
| `WithBufferSize`        | In-memory queue capacity (default 10000)                   |
| `WithBatchSize`         | Max batch size passed to `Sink.StoreBatch` (default 100)   |
| `WithFlushInterval`     | Partial-batch flush deadline (default 1 s)                 |
| `WithWorkers`           | Number of dispatch goroutines (default 2)                  |
| `WithRetryAttempts`     | Max retries per failed batch (default 3)                   |
| `WithRetryBackoff`      | Exponential backoff base (default 100 ms)                  |
| `WithBackPressure`      | Block `Submit` when the buffer is full instead of dropping |
| `WithOnDrop`            | Callback invoked on submit-time drop                       |
| `WithLogger`            | `*slog.Logger` for diagnostic messages                     |
| `WithCollector`         | `metrics.Collector` (metrics emitted under the subsystem)  |
| `WithMetricsSubsystem`  | Override the metrics subsystem name (default `"async"`)    |
| `WithWAL`               | Enable crash-safe WAL; requires a `Codec[T]`               |

> The default metrics subsystem is `"async"` (not `"dispatch"`) so
> dashboards and alerts built against the earlier location of this
> package keep working after the rename. Override it via
> `WithMetricsSubsystem` when embedding the engine inside a facade.

## WAL

The write-ahead log lives in its own package,
[`core/io/wal`](../../../io/wal), and can be used independently of this
engine. `WithWAL(dir, codec, walOpts...)` wires it in; the full API
surface, durability guarantees, and performance notes are documented
in the [`core/io/wal` README](../../../io/wal/README.md). Only the
`wal.Option` variadic tail is exposed through this package — everything
else is the WAL package's surface.

## Usage

```go
sink := &mySink{}          // implements dispatch.Sink[*Event]
codec := myCodec{}         // implements dispatch.Codec[*Event]

eng, err := dispatch.NewEngine[*Event](sink,
    dispatch.WithBatchSize[*Event](100),
    dispatch.WithFlushInterval[*Event](time.Second),
    dispatch.WithWAL[*Event]("/var/lib/svc/wal", codec,
        wal.WithMaxSegmentBytes(64<<20),
        wal.WithFsyncInterval(5*time.Millisecond),
    ),
)
if err != nil {
    return err
}
if err := eng.Start(); err != nil {
    return err
}
defer func() {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    _ = eng.Shutdown(ctx)
}()

eng.Submit(&Event{...}) // non-blocking hot path
```

For a runnable end-to-end example, see `Example` in `example_test.go`.

## When to use — and when not to

Use `dispatch.Engine` when you need a non-blocking producer interface
with batching and optional local durability. Prefer a dedicated facade
for domain-specific producers (audit, request logs) over a direct
dependency on this package; build a new facade only when the schema and
sink semantics genuinely differ.

Do **not** use it as a distributed queue: delivery is to a single
in-process `Sink`, and the WAL is a local, per-process spool. Cross-node
durability belongs to the broker layer.
