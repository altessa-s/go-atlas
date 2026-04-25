# pbwrap

```go
import "github.com/altessa-s/go-atlas/domain/converter/codec/pbwrap"
```

Package `pbwrap` provides a codec for bidirectional conversion between protobuf wrapper types
(`wrapperspb.*`) and primitive Go types.

## Supported wrappers

`StringValue`, `Int32Value`, `Int64Value`, `UInt32Value`, `UInt64Value`, `BoolValue`, `FloatValue`,
`DoubleValue`. `BytesValue` is excluded because `[]byte` requires special handling.

## Options

| Option                | Description                             |
|-----------------------|-----------------------------------------|
| `WithIgnoreZeroValues`| Skip conversion when value is zero      |
