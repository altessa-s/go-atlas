# spool

```go
import "github.com/altessa-s/go-atlas/core/io/spool"
```

Materializes a streaming `io.Reader` into a local, rewindable backing store. Content up to `DefaultMemThreshold` (4 MiB) is held in a byte slice; larger content spills to a temp file. Callers can re-read from offset 0 without touching the origin again.

## Options

| Option | Default | Description |
|--------|---------|-------------|
| `WithMemThreshold(n int64)` | `4 MiB` | Maximum bytes to hold in memory before spilling to a temp file |
| `WithMaxBytes(n int64)` | `0` (unlimited) | Hard cap on total materialized size; returns `ErrTooLarge` when exceeded |
| `WithTee(w io.Writer)` | `nil` | Mirror every byte read from the source into w during materialization |

## Key types

| Symbol | Description |
|--------|-------------|
| `Spool` | Rewindable, closeable backing store |
| `Option` | Functional option for `New` |
| `ErrTooLarge` | Sentinel returned when source exceeds `WithMaxBytes` |

## Usage

```go
// Read a remote body, compute its hash, re-read for processing.
h := sha256.New()
sp, err := spool.New(body,
    spool.WithTee(h),
    spool.WithMaxBytes(32<<20),
)
if err != nil {
    return err
}
defer sp.Close()

digest := h.Sum(nil)

// sp is now at offset 0 — read as many times as needed.
data, err := io.ReadAll(sp)
```
