# slog

```go
import slogx "github.com/altessa-s/go-atlas/observability/slog"
```

Package `slog` provides nil-safe attribute helpers, context integration, and composable handlers for Go's `log/slog`. Import with alias
`slogx` to avoid conflict with the standard library. All helpers are safe for concurrent use.

## Functions

| Function / Type       | Description                                  |
|-----------------------|----------------------------------------------|
| `String`, `Int`, etc. | Nil-safe attribute helpers for pointer types |
| `Error`               | Nil-safe error attribute                     |
| `ContextWithLogger`   | Store a logger in context                    |
| `FromContext`         | Retrieve a logger from context               |
| `GlobalLevel`         | Runtime log level management                 |
| `Shutdown`            | Graceful shutdown for handlers that buffer   |

## Subpackages

| Package                                         | Description                                        |
|-------------------------------------------------|----------------------------------------------------|
| [factory](./factory)                            | Configuration-based `slog.Logger` creation         |
| [handler/buffered](./handler/buffered)          | Async buffered handler with bypass level           |
| [handler/colorized](./handler/colorized)        | Color-coded terminal output for development        |
| [handler/leveled](./handler/leveled)            | Per-subsystem log level filtering                  |
| [handler/masking](./handler/masking)            | PII and credential masking in log records          |
| [handler/multi](./handler/multi)                | Fan-out to multiple handlers (stdlib on Go 1.26+)  |
| [handler/prefixed](./handler/prefixed)          | Middleware that adds component prefixes            |
