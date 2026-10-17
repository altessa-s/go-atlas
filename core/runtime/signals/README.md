# signals

```go
import "github.com/altessa-s/go-atlas/core/runtime/signals"
```

Package `signals` provides OS signal handling with priority-based execution and graceful shutdown. Fully thread-safe.

## Options

| Option                | Default    | Description                          |
|-----------------------|------------|--------------------------------------|
| `WithSignals`         | —          | OS signals to listen for             |
| `WithWorkerPoolSize`  | 10         | Max concurrent handler goroutines    |
| `WithShutdownTimeout` | 30s        | Graceful shutdown deadline           |
| `WithHandlerTimeout`  | 5s         | Per-handler execution timeout        |
| `WithExecutionMode`   | Sequential | `SequentialMode` or `ParallelMode`   |
| `WithErrorHandler`    | —          | Callback for handler errors/panics   |
| `WithSignalChannelBuffer` | 1      | Buffer size of the OS signal channel |

## Signal methods

`New(opts...)` returns a `*Signal`; registration and start methods return the `SignalHandler` interface for chaining.

| Method                                          | Description                                                       |
|-------------------------------------------------|-------------------------------------------------------------------|
| `AddHandler(h, sigs...)`                        | Register a `ContextHandler` at `PriorityNormal` for given signals |
| `AddHandlerWithPriority(h, p, sigs...)`         | Register a handler with an explicit `Priority`                    |
| `AddBroadcastHandler(h)`                        | Register a handler for every received signal                      |
| `AddBroadcastHandlerWithPriority(h, p)`         | Broadcast handler with an explicit `Priority`                     |
| `Start()`                                       | Begin listening for the configured signals (idempotent)           |
| `Shutdown(ctx)` / `Stop()`                      | Stop listening and wait for in-flight handlers (idempotent)       |
| `Wait()`                                        | Block until the handler has stopped                               |

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
