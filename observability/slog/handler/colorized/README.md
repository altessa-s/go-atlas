# colorized

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/colorized"
```

Package `colorized` provides a color-coded terminal `slog.Handler` for human-readable log output. Designed for CLI and development use.

## Options

| Option                 | Description                                     |
|------------------------|-------------------------------------------------|
| `WithLevel`            | Minimum log level                               |
| `WithAddSource`        | Include source file and line                    |
| `WithNoColor`          | Disable ANSI colors                             |
| `WithTimeFormat`       | Custom time format string                       |
| `WithReplaceAttr`      | Attribute transformation function               |
| `WithAttributeColors`  | Custom color map for attribute keys             |
