# time

```go
import "github.com/altessa-s/go-atlas/core/time"
```

Package `time` provides safe timer management utilities. Extends the standard library with helpers to prevent goroutine leaks by safely
stopping timers and draining their channels. All functions are thread-safe, nil-safe, and idempotent.

## Timer utilities

| Function            | Description                                               |
|---------------------|-----------------------------------------------------------|
| `TimerStopAndDrain` | Stop a `*time.Timer` and drain its channel (non-blocking) |

Returns `true` if the timer was stopped before firing, `false` if it had already expired or `t` is nil. Safe to call on an already-stopped timer.

## Subpackages

| Package                    | Description                                        |
|----------------------------|----------------------------------------------------|
| [timeformat](./timeformat) | RFC 3339 and Unix timestamp formatting and parsing |
