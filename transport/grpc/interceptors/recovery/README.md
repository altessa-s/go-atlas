# recovery

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/recovery"
```

Package `recovery` provides gRPC interceptors for recovering from panics in both server and client calls. Configurable panic handlers, stack trace
capture with goroutine ID, and request ID correlation. Returns `codes.Internal` by default. Integrates with the
[`go-atlas-core/runtime/panics`](https://github.com/altessa-s/go-atlas-core/tree/develop/runtime/panics) package.

## Key types

| Type / Interface | Description                                            |
|------------------|--------------------------------------------------------|
| `PanicHandler`   | `func(ctx, p any) error` -- custom panic handling      |

## Options

| Option               | Default            | Description                                       |
|----------------------|--------------------|---------------------------------------------------|
| `WithPanicHandler`   | default handler    | Custom panic handling function                    |
| `WithLogger`         | discard            | Structured logger for panic details               |
| `WithIgnoreMethods`  | --                 | Methods to skip panic recovery                    |
| `WithIgnorePatterns` | --                 | Regex patterns for methods to skip                |

Recovery covers every method by default, including the gRPC health and reflection services.
