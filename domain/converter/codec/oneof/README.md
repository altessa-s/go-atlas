# oneof

```go
import "github.com/altessa-s/go-atlas/domain/converter/codec/oneof"
```

Package `oneof` provides a codec for bidirectional conversion between struct-based oneof representations and protobuf oneof interface types.

## Directions

- Struct with optional pointer fields -> protobuf oneof interface wrapper
- Protobuf oneof interface wrapper -> struct with optional pointer fields

## Constructors

| Function      | Description                                     |
|---------------|-------------------------------------------------|
| `New`         | Create a global oneof codec                     |
| `NewForField` | Create a codec scoped to a specific field name  |

## Options

| Option                 | Description                                                          |
|------------------------|----------------------------------------------------------------------|
| `WithWrapperRegistry`  | Field name → wrapper prototype map; required for struct → oneof      |
| `WithFieldConverter`   | Custom `FieldConverter` for one field                                |
| `WithStrictValidation` | Pass to the next codec when more than one oneof field is non-nil     |
| `WithIgnoreNilFields`  | Leave the destination unchanged and stop the chain when every field is nil and no fallback is set |
| `WithFallbackField`    | Variant to set when every oneof field is nil                         |

Option and registry lookups are case-insensitive; the struct and wrapper field names themselves must match case exactly.
