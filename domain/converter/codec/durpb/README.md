# durpb

```go
import "github.com/altessa-s/go-atlas/domain/converter/codec/durpb"
```

Package `durpb` provides a codec for bidirectional conversion between `durationpb.Duration` and
`time.Duration`.

## Options

| Option           | Description                               |
|------------------|-------------------------------------------|
| `WithIgnoreZero` | Skip conversion for zero-value durations  |

Both pointer and non-pointer variants are supported. Uses unsafe pointer arithmetic for zero-allocation
reads/writes when the underlying `reflect.Value` is addressable.

`time.Duration` <-> `int64` is intentionally not handled: `time.Duration` has an `int64` kind, so the
converter's built-in convertible path already maps it without a codec.
