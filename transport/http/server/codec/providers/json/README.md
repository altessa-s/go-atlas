# json

```go
import "github.com/altessa-s/go-atlas/transport/http/server/codec/providers/json"
```

Package `json` provides a JSON `codec.Codec` and `codec.StreamingEncoder` backed by `encoding/json`. The codec is safe for concurrent use
and supports two MIME types: `application/json` (primary) and `text/json` (alternate). Both are registered in the global `DefaultRegistry`
at init time. HTML escaping is enabled by default.

## Options

| Option           | Default | Description                                                                          |
|------------------|---------|--------------------------------------------------------------------------------------|
| `WithIndent`     | false   | Enable pretty-printed JSON output with indentation                                   |
| `WithEscapeHTML` | true    | Enable HTML escaping of `<`, `>`, and `&` characters in JSON strings                 |
