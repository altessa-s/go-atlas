# wal

```go
import "github.com/altessa-s/go-atlas/core/io/wal"
```

Package `wal` implements a segmented, append-only write-ahead log for
crash-safe durability of opaque byte payloads. Records are length-prefixed,
CRC32-protected, and fsynced on a configurable interval. The package is
payload-agnostic — higher-level components (dispatchers, outboxes, durable
queues) layer their own encoding and semantics on top.

## Lifecycle

| Step | Call |
|------|------|
| Open or recover | `wal.Open(dir, opts...)` |
| Replay each `Record` | `process(r.Payload) → w.Ack(r.Offset)` |
| Hot path | `off, err := w.Append(payload)` then later `w.Ack(off)` |
| Force flush | `w.Sync()` |
| Shutdown | `w.Close()` — aggregates final Sync/Close errors via `errors.Join` |

Delivery is **at-least-once**: records remain on disk until acked, and
unacked records from sealed segments (or the active segment at crash
time) are returned by the next `Open` call. Consumers must be idempotent
with respect to replayed payloads.

## Options

| Option                  | Default                    | Description                                                                 |
|-------------------------|----------------------------|-----------------------------------------------------------------------------|
| `WithMaxSegmentBytes`   | `DefaultMaxSegmentBytes` (64 MiB) | Roll-over size for a single segment file                             |
| `WithMaxBytes`          | `DefaultMaxBytes` (0 = unlimited) | Soft cap on total bytes across all segments; `Append` returns `ErrFull` when exceeded |
| `WithFsyncInterval`     | `DefaultFsyncInterval` (5 ms)     | Period between background fsync calls on the active segment          |
| `WithLogger`            | `slog.DiscardHandler`      | `*slog.Logger` used to surface background fsync errors                      |

## Types

| Type     | Description                                                         |
|----------|---------------------------------------------------------------------|
| `WAL`    | Segmented append-only log; safe for concurrent `Append`/`Ack`/`Sync`/`Stats`/`Close` |
| `Offset` | `{SegmentID, Position}` identifier; zero value means "no record"    |
| `Record` | `{Offset, Payload}` returned from `Open` during recovery            |
| `Stats`  | `{TotalBytes, Segments}` snapshot for metrics                       |

## Sentinel errors

| Error       | Meaning                                                        |
|-------------|----------------------------------------------------------------|
| `ErrClosed` | Returned by `Append` and other mutating ops after `Close`      |
| `ErrFull`   | Returned by `Append` when the `WithMaxBytes` soft cap is exceeded |

## Durability guarantees

| Scenario                      | Guarantee                                                    |
|-------------------------------|--------------------------------------------------------------|
| Graceful `Close`              | Final fsync + close; acked active segment is removed on disk |
| `SIGKILL` / panic / OOM       | Records past the last fsync survive and replay on next `Open` |
| Loss window                   | Up to `WithFsyncInterval` (default 5 ms)                     |
| Torn write on crash           | Detected via length+CRC32, truncated during recovery         |
| Sink temporarily unavailable  | Records accumulate up to `WithMaxBytes` then `ErrFull`       |

## Usage

```go
w, recovered, err := wal.Open("/var/lib/app/wal",
    wal.WithMaxSegmentBytes(16<<20),
    wal.WithFsyncInterval(2*time.Millisecond),
    wal.WithLogger(logger),
)
if err != nil {
    return err
}
defer func() { _ = w.Close() }()

// Replay any records left over from a previous run.
for _, r := range recovered {
    if err := process(r.Payload); err == nil {
        w.Ack(r.Offset)
    }
}

// Hot path.
off, err := w.Append(payload)
if err != nil {
    return err
}
// ... eventually, after the payload is durably handled downstream:
w.Ack(off)
```

For a runnable example see `wal_test.go → TestWAL_Recovery_ReplaysUnackedRecords`
and `BenchmarkWAL_OpenRecover` in `wal_bench_test.go`.

## Performance notes

- `Append` serializes on an internal mutex during the `file.Write`
  syscall to keep on-disk offsets and accounting consistent. On a slow
  disk this limits per-process Append throughput to the disk's write
  latency — batched upstream producers (e.g.
  [`core/runtime/concurrency/dispatch`](../../runtime/concurrency/dispatch))
  hide this by accumulating in memory before calling `Append`.
- Header and payload are coalesced into a single scratch buffer so
  each `Append` issues exactly one `write` syscall.
- `Ack` is O(N) over sealed segments; the expected N is 1–2 in typical
  dispatcher configurations.
- The background fsync goroutine holds `w.mu` only long enough to
  read `w.active.file`; it does not block Append for the duration of
  the `Sync` syscall.

## When to use — and when not to

Use `wal` directly when you need a **local, per-process durable spool**
for opaque byte payloads — e.g. an outbox, a crash-safe audit buffer, a
replayable command log. The package does not handle encoding, batching,
or consumer back-pressure.

For the common "batching async dispatcher with optional WAL durability"
pattern, use
[`core/runtime/concurrency/dispatch`](../../runtime/concurrency/dispatch) —
it wraps this package behind a `Sink[T]` + `Codec[T]` interface and
handles replay, retries, metrics, and graceful shutdown.

**Do not** use `wal` as a distributed log: it is a single-process append
log on a local filesystem. Cross-node durability belongs to the broker
layer.
