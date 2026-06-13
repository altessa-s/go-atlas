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

## RangeReadSeeker

Adapts a `RangeOpener` — any "open a ranged read at offset" function (S3 `GetObject` with a `Range` header, an HTTP Range request) — to
`io.ReadSeekCloser` for consumers that need random access over a remote object without downloading it up front. Reads lazily open a
ranged request at the current position; `Seek` only moves the position and forces the next `Read` to reopen. A body shorter than the
requested range surfaces as `io.ErrUnexpectedEOF`. Not safe for concurrent use; the caller must `Close` it to release the open body.

## Subpackages

| Package          | Description                                                                |
|------------------|----------------------------------------------------------------------------|
| [files](./files) | File/directory existence checks, multi-path search                         |
| [wal](./wal)     | Segmented append-only write-ahead log with CRC32, fsync policy, recovery   |
