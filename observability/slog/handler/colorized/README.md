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

## Attribute keys

Attribute keys are quoted and escaped when they contain whitespace, control characters, quotes, equals signs or invalid UTF-8.
This applies to the complete flattened group path, including `WithGroup` and `WithAttrs`, before ANSI color is added. Ordinary keys keep
their existing format and color selection uses the original key.
