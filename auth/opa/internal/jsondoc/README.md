# jsondoc

```go
import "github.com/altessa-s/go-atlas/auth/opa/internal/jsondoc"
```

Package `jsondoc` decodes the JSON documents shared by policy sources.

## Usage

`Decode(content)` reads one JSON value, preserves numbers as `json.Number`, and rejects trailing tokens.
