# io

```go
import "github.com/altessa-s/go-atlas/core/io"
```

Package `io` provides I/O utilities including buffer pooling, size-limited readers, and test helpers for simulating read errors.

## Buffer pool

`GetBuffer` / `PutBuffer` wrap a `sync.Pool` of `*bytes.Buffer`. Buffers exceeding 64 KB are discarded on return to prevent unbounded
memory retention in long-running services. Typical usage: borrow via `GetBuffer`, write, then `PutBuffer`.

## LimitedReadCloser

Wraps an `io.ReadCloser` with a byte-count limit. Returns `ErrReadLimitExceeded` when the limit is exceeded instead of silently
truncating the data. Concurrent-safe — `Close` can be called from multiple goroutines without data races.

## ErrorReader

An `io.Reader` that always fails with a given error. Useful for tests that need to simulate I/O failures without hitting the file system.

## Subpackages

| Package          | Description                                                                |
|------------------|----------------------------------------------------------------------------|
| [files](./files) | File/directory existence checks, multi-path search                         |
| [wal](./wal)     | Segmented append-only write-ahead log with CRC32, fsync policy, recovery   |
