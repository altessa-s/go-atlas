# oneof

```go
import "github.com/altessa-s/go-atlas/domain/converter/codec/oneof"
```

Package `oneof` provides a codec for bidirectional conversion between struct-based oneof representations
and protobuf oneof interface types.

## Directions

- Struct with optional pointer fields -> protobuf oneof interface wrapper
- Protobuf oneof interface wrapper -> struct with optional pointer fields

## Constructors

| Function      | Description                                     |
|---------------|-------------------------------------------------|
| `New`         | Create a global oneof codec                     |
| `NewForField` | Create a codec scoped to a specific field name  |

Field matching is case-insensitive.
