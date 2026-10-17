# dispatch

```go
import "github.com/altessa-s/go-atlas/service/dispatch"
```

Package `dispatch` provides a generic, non-blocking, batching dispatch engine with optional crash-safe persistence backed by
[`core/io/wal`](../../core/io/wal).

It is the foundation for fire-and-forget producers (audit logs, request access logs, telemetry) where the hot path must not block on I/O but events must
still survive process crashes.

## Hot path

Without a WAL, `Engine.Submit` only sends the item to an in-memory channel and returns. With `WithWAL` enabled, `Submit` additionally serializes the
item via the configured `Codec`, appends it to the active WAL segment (page-cache write, no fsync) and notifies a durable worker, which reads pending
records straight from the WAL. A background goroutine fsyncs the active segment every fsync interval (group commit), bounding the loss window after a
crash.

Workers (durable ones from the WAL, volatile ones from the in-memory channel) build batches up to `BatchSize` or `FlushInterval`, and call
`Sink.StoreBatch`. On success the corresponding WAL records are acked; once all records in a sealed segment are acked, the segment file is removed.
Sinks receive **at-least-once** delivery: a single item may reappear after a crash-and-replay sequence, so implementations should be idempotent on the
records' natural identity (e.g. event ID).

Acks live only in memory, and a segment is removed only once every record in it is acked. When `Shutdown` returns `ErrBacklog` (the sink kept failing or
the deadline ran out), `Start` on a new engine using the same WAL directory therefore replays every segment that still holds an undelivered record —
including records of those segments that were already delivered and acked. The same holds after a crash for the active segment, which is removed only
when it rolls over or at `Close`.

## Durability guarantees

| Scenario                      | Guarantee                                                    |
|-------------------------------|--------------------------------------------------------------|
| Graceful `Shutdown(ctx)`      | Full drain to `Sink`, acked segments removed                 |
| `Shutdown` with a backlog     | `ErrBacklog`; a new engine on the WAL replays whole segments |
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
| `Engine.Enqueue`     | Like `Submit`, but reports the tier that accepted the item (`Rejected`, `Queued`, `Logged`) and the error |
| `Engine.Shutdown`    | Drain pending items and close the WAL, bounded by `ctx`    |
| `Engine.Dropped`     | Total items dropped at submit time                         |
| `Engine.WAL`         | Underlying `*wal.WAL`, or nil when WAL is disabled         |

## Errors

| Sentinel          | Meaning                                                    |
|-------------------|------------------------------------------------------------|
| `ErrEngineClosed` | The engine is shut down and accepts no more items          |
| `ErrQueueFull`    | The volatile queue has no capacity                         |
| `ErrBacklog`      | `Shutdown` left accepted records undelivered               |
| `ErrReplayDecode` | Replay cannot decode a retained WAL record                 |
| `ErrAlreadyStarted` | `Start` was called on an engine that already started     |
| `ErrSinkRequired` | `NewEngine` was given a nil sink                           |
| `ErrCodecRequired` | The WAL is enabled but no codec is set                    |

## Options

| Option                  | Effect                                                     |
|-------------------------|------------------------------------------------------------|
| `WithBufferSize`        | In-memory queue capacity (default 10000)                   |
| `WithBatchSize`         | Max batch size passed to `Sink.StoreBatch` (default 100)   |
| `WithFlushInterval`     | Partial-batch flush deadline (default 1 s)                 |
| `WithWorkers`           | Number of dispatch goroutines (default 2)                  |
| `WithRetryAttempts`     | Max retries per failed batch (default 3)                   |
| `WithRetryBackoff`      | Exponential backoff base (default 100 ms)                  |
| `WithRetryMaxBackoff`   | Cap on retry waits during a durable sink outage (default 30 s) |
| `WithRetryJitter`       | Fraction of each backoff added as random jitter (default 0.2) |
| `WithStoreTimeout`      | Deadline for each `Sink.StoreBatch` call (default 30 s)    |
| `WithBackPressure`      | Block `Submit` when the buffer is full instead of dropping |
| `WithOnDrop`            | Callback invoked on submit-time drop                       |
| `WithLogger`            | `*slog.Logger` for diagnostic messages                     |
| `WithCollector`         | `metrics.Collector` (metrics emitted under the subsystem)  |
| `WithMetricsSubsystem`  | Override the metrics subsystem name (default `"async"`)    |
| `WithWAL`               | Enable crash-safe WAL; requires a `Codec[T]`               |

> The default metrics subsystem is `"async"` (not `"dispatch"`) so dashboards and alerts built against the earlier location of this package keep working
> after the rename. Override it via `WithMetricsSubsystem` when embedding the engine inside a facade.

## WAL

The write-ahead log lives in its own package, [`core/io/wal`](../../core/io/wal), and can be used independently of this engine.
`WithWAL(dir, codec, walOpts...)` wires it in; the full API surface, durability guarantees, and performance notes are documented in the
[`core/io/wal` README](../../core/io/wal/README.md). Only the `wal.Option` variadic tail is exposed through this package — everything else is the WAL
package's surface.

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

Use `dispatch.Engine` when you need a non-blocking producer interface with batching and optional local durability. Prefer a dedicated facade for
domain-specific producers (audit, request logs) over a direct dependency on this package; build a new facade only when the schema and sink semantics
genuinely differ.

Do **not** use it as a distributed queue: delivery is to a single in-process `Sink`, and the WAL is a local, per-process spool. Cross-node durability
belongs to the broker layer.
