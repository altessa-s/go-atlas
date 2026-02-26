# prefixed

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/prefixed"
```

Package `prefixed` provides a `slog.Handler` middleware that adds configurable prefixes to log messages.
Useful for categorizing logs by component or subsystem.

## Usage

```go
h := prefixed.NewHandler(inner,
    prefixed.WithPrefix("auth"),
)
logger := slog.New(h)
logger.Info("login attempt") // output: "[auth] login attempt"
```

## Options

| Option                 | Description                                  |
|------------------------|----------------------------------------------|
| `WithPrefix`           | Set the prefix string                        |
| `WithPrefixFormatter`  | Custom format function for the prefix        |
| `DefaultFormatter`     | Built-in bracket formatter `[prefix]`        |
| `JsonFormatter`        | Built-in JSON-style prefix formatter         |
