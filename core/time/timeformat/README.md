# timeformat

```go
import "github.com/altessa-s/go-atlas/core/time/timeformat"
```

Package `timeformat` provides time formatting and parsing with RFC 3339 and Unix timestamp support at multiple precisions.

## Formats

| Constant      | Layout / Precision                             |
|---------------|------------------------------------------------|
| `RFC3339`     | `2006-01-02T15:04:05Z07:00` (second precision) |
| `RFC3339Nano` | `2006-01-02T15:04:05.999999999Z07:00`          |
| `Unix`        | Seconds since epoch                            |
| `UnixMilli`   | Milliseconds since epoch                       |
| `UnixMicro`   | Microseconds since epoch                       |
| `UnixNano`    | Nanoseconds since epoch                        |

## Functions

| Function / Method | Description                                                            |
|-------------------|------------------------------------------------------------------------|
| `Format.Format`   | Render `time.Time` as string                                           |
| `Format.Parse`    | Parse string into `time.Time`                                          |
| `FormatTime`      | Format as `any` — string for RFC layouts, `int64` for Unix formats     |
| `FormatDuration`  | Format `time.Duration` — always nanoseconds `int64` for every Unix format (the format does not select the resolution), as-is otherwise |
