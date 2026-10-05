# prefixed

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/prefixed"
```

Package `prefixed` provides a `slog.Handler` middleware that adds configurable prefixes to log messages. Useful for categorizing logs
by component or subsystem. String attributes under the configured key are merged into one formatted attribute under the same key. Ships
with bracket and JSON formatters; custom formatters are supported.

## Options

| Option                  | Default            | Description                                                |
|-------------------------|--------------------|------------------------------------------------------------|
| `WithPrefix`            | empty (required)   | Attribute key whose string values become prefixes          |
| `WithPrefixFormatter`   | `DefaultFormatter` | Function turning the prefix values + delimiter into a value |
| `WithPrefixesDelimiter` | `":"`              | Separator between multiple prefix values                   |

## Formatters

| Formatter          | Output         |
|--------------------|----------------|
| `DefaultFormatter` | `[api:server]` |
| `JsonFormatter`    | `api:server`   |

## Usage

```go
logger := slog.New(prefixed.NewHandler(slog.NewTextHandler(os.Stdout, nil), prefixed.WithPrefix("module")))
logger.With("module", "api").Info("started", "module", "server") // msg=started module=[api:server]
```

## Groups

A prefix stays at the group level it was attached to: prefixes collected at one level are merged and written at that level when the next
group is opened, and the new group starts with no prefixes. `logger.With("module", "api").WithGroup("req").Info("m", "id", 1)` renders
`module=[api] req.id=1` (JSON: `{"module":"api","req":{"id":1}}`). Within a level the prefix attribute follows that level's other
attributes. Non-string attributes under the prefix key are passed through untouched.
