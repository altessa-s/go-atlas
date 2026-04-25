# unixtime

```go
import "github.com/altessa-s/go-atlas/domain/converter/codec/unixtime"
```

Package `unixtime` provides a codec for bidirectional conversion between `time.Time` and `int64` Unix
timestamps with optional millisecond precision.

## Options

| Option              | Description                                      |
|---------------------|--------------------------------------------------|
| `WithIgnoreZero`    | Skip conversion for zero-value timestamps        |
| `WithMilliseconds`  | Use millisecond precision (default is seconds)   |

Both pointer and non-pointer variants are supported.
