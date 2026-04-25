# signals

```go
import "github.com/altessa-s/go-atlas/core/runtime/signals"
```

Package `signals` provides OS signal handling with priority-based execution, rate limiting, and graceful shutdown. Fully thread-safe.

## Options

| Option                | Default    | Description                          |
|-----------------------|------------|--------------------------------------|
| `WithSignals`         | —          | OS signals to listen for             |
| `WithWorkerPoolSize`  | 10         | Max concurrent handler goroutines    |
| `WithShutdownTimeout` | 30s        | Graceful shutdown deadline           |
| `WithHandlerTimeout`  | 5s         | Per-handler execution timeout        |
| `WithExecutionMode`   | Sequential | `SequentialMode` or `ParallelMode`   |
| `WithErrorHandler`    | —          | Callback for handler errors/panics   |

## Priority levels

| Constant          | Value | Use case                          |
|-------------------|-------|-----------------------------------|
| `PriorityHighest` | 100   | Critical system operations        |
| `PriorityHigh`    | 75    | Important business logic          |
| `PriorityNormal`  | 50    | Default                           |
| `PriorityLow`     | 25    | Background tasks                  |
| `PriorityLowest`  | 1     | Logging, metrics                  |

## Error types

| Type            | Description                                          |
|-----------------|------------------------------------------------------|
| `*TimeoutError` | Handler exceeded its timeout; check with `IsTimeout` |
| `*PanicError`   | Handler panicked; recovered value in `Panic` field   |
