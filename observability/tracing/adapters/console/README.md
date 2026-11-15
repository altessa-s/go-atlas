# console

```go
import "github.com/altessa-s/go-atlas/observability/tracing/adapters/console"
```

Package `console` provides a console adapter for the Atlas tracing system. Outputs spans to stdout or stderr in human-readable or JSON format. Designed
for local development and debugging -- not recommended for production use.

## Options

| Option                  | Default     | Description                                                           |
|-------------------------|-------------|-----------------------------------------------------------------------|
| `WithWriter(io.Writer)` | `os.Stdout` | Output destination                                                    |
| `WithPrettyPrint(bool)` | `true`      | `true` prints human-readable lines; `false` prints one JSON object    |
| `WithTimestamps(bool)`  | `true`      | Prefixes each human-readable line with a timestamp (pretty mode only) |
