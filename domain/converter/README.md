# converter

```go
import "github.com/altessa-s/go-atlas/domain/converter"
```

Package `converter` provides high-performance struct-to-struct conversion with field mapping, filtering,
embedded structs, slices, maps, and protocol buffers. It caches type metadata and pools allocations for
optimal throughput.

## Options

| Option                       | Description                                       |
|------------------------------|---------------------------------------------------|
| `WithCodecs`                 | Register custom codec pipeline                    |
| `WithIgnoreFields`           | Skip specific fields by name (case-insensitive)   |
| `WithFieldMappings`          | Map source field names to destination names        |
| `WithIgnoreZeroValues`       | Skip zero-valued source fields                    |
| `WithIgnoreNilValues`        | Skip nil source fields (pointers, slices, maps)   |
| `WithHandleEmbeddedStructs`  | Recursively expand embedded (anonymous) structs   |
| `WithOverflowCheck`          | Panic on narrowing numeric overflow               |

## Functions

| Function        | Description                                            |
|-----------------|--------------------------------------------------------|
| `Convert`       | One-shot struct/slice conversion                       |
| `New`           | Create reusable generic `Converter[T, U]`              |
| `NewAny`        | Create reusable non-generic `Converter[any, any]`      |
| `ConvertSeq`    | Lazy iterator over converted slice elements            |
| `ConvertMapSeq` | Lazy iterator over converted map key-value pairs       |
| `IndirectType`  | Dereference all pointer layers from a `reflect.Type`   |

## Subpackages

| Package                                          | Description                                          |
|--------------------------------------------------|------------------------------------------------------|
| [codec](./codec)                                 | Pluggable codec interface and chain execution engine |
| [codecs/optionalcodec](./codecs/optionalcodec)   | `core/types/optional.Optional[T]` field codec        |
