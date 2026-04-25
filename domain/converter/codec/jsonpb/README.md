# jsonpb

```go
import "github.com/altessa-s/go-atlas/domain/converter/codec/jsonpb"
```

Package `jsonpb` provides a codec for bidirectional conversion between `json.RawMessage` and
`structpb.Struct`.

## Options

| Option          | Description                                |
|-----------------|--------------------------------------------|
| `WithIgnoreNil` | Skip conversion when the source is nil     |

Both pointer and non-pointer variants are supported.
