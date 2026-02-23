# io

```go
import "github.com/altessa-s/go-atlas/core/io"
```

Package `io` provides I/O utilities including buffer pooling and size-limited readers.

## Buffer pool

`GetBuffer` / `PutBuffer` wrap a `sync.Pool` of `*bytes.Buffer`. Buffers exceeding 64 KB are discarded on return to prevent unbounded memory retention.

```go
buf := io.GetBuffer()
defer io.PutBuffer(buf)
buf.WriteString("hello")
```

## LimitedReadCloser

Wraps an `io.ReadCloser` with a byte-count limit. Returns `ErrReadLimitExceeded` when the limit is exceeded instead of silently truncating. Concurrent-safe.

```go
lr := io.NewLimitedReadCloser(rc, 1<<20) // 1 MB
defer lr.Close()
```

## ErrorReader

An `io.Reader` that always fails with a given error. Useful for tests.

```go
r := io.NewErrorReader(errSimulated)
```

## Subpackages

| Package          | Description                                          |
|------------------|------------------------------------------------------|
| [files](./files) | File/directory existence checks, multi-path search   |
