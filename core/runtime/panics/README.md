# panics

```go
import "github.com/altessa-s/go-atlas/core/runtime/panics"
```

Package `panics` provides panic recovery with pluggable handlers and runtime assertion helpers. All configuration uses atomic operations
and is thread-safe — handlers can be registered and modified concurrently without external locks.

## Recovery

| Function           | Description                                              |
|--------------------|----------------------------------------------------------|
| `Handle`           | Recover and run global + per-call handlers               |
| `HandleWithOpts`   | Same with per-call options (e.g., override re-panic)     |

## Guarded channel sends

| Function             | Description                                                                |
|----------------------|----------------------------------------------------------------------------|
| `TrySend`            | Blocking send, canceled by `ctx`; recovers the send-on-closed panic        |
| `TrySendNonBlocking` | Select-with-default send; recovers the send-on-closed panic                |

Both report `(sent, closed bool)`. Needing them is a channel-ownership smell — the sender should own the close — but they contain the
crash when that ownership is shared or inverted.

## Assertions

| Function          | Panics when                                      |
|-------------------|--------------------------------------------------|
| `Must`            | `ok` is `false`                                  |
| `MustNonNil`      | value is `nil`                                   |
| `MustNonZero`     | value is `nil` or zero value                     |
| `MustError`       | `err` is non-nil                                 |
| `MustResult`      | `err` is non-nil; otherwise returns `T`          |
| `InvalidArgument` | condition is `true` (wraps `ErrInvalidArgument`) |

## Configuration

| Function                 | Description                                         |
|--------------------------|-----------------------------------------------------|
| `SetReallyPanic`         | Re-panic after handlers (default: `false`)          |
| `SetLogger`              | Set `slog.Logger` for the built-in handler          |
| `SetLoggerFromContext`   | Extract logger from context in the built-in handler |
| `AddGlobalPanicHandler`  | Append a handler to the global list                 |
| `SetGlobalPanicHandlers` | Replace all global handlers                         |
