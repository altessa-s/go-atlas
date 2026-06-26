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

## Spool

Materializes an `io.Reader` exactly once into a local, rewindable `io.ReadSeekCloser` — in memory for small content, in a temp file once it
exceeds the memory threshold — so consumers can re-read it from offset 0 without re-touching the (possibly remote) origin. Options:
`WithSpoolMemThreshold` overrides the memory/file cutoff (`DefaultSpoolMemThreshold`, 4 MiB); `WithSpoolMaxBytes(n)` caps the size and
returns `ErrSpoolTooLarge` past it (`n <= 0` is unlimited); `WithSpoolTee(w)` mirrors every byte into `w` during the single pass so a digest
(e.g. SHA-256) can be computed without a second read. `Size` reports the materialized length. The source is never closed (caller owns it);
`Close` removes a spilled temp file and is idempotent. Not safe for concurrent use.

## Subpackages

| Package                      | Description                                                                |
|------------------------------|----------------------------------------------------------------------------|
| [files](./files)             | File/directory existence checks, multi-path search                         |
| [spool](./spool)             | Materialize a reader into a rewindable in-memory/temp-file backing store    |
| [spoolbudget](./spoolbudget) | Process-wide disk ceiling charged across concurrent `Spool` reservations   |
| [wal](./wal)                 | Segmented append-only write-ahead log with CRC32, fsync policy, recovery   |
