# codec

```go
import convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
```

Package `convcodec` provides the codec interface and chain execution engine for the converter. Individual
codec implementations live in subpackages.

A `Codec` either handles the conversion and returns, or delegates to `next`. Codecs execute in registration
order inside a `Set`.

## Set

| Method      | Description                                     |
|-------------|-------------------------------------------------|
| `NewCodecsSet` | Create a `Set` with initial codecs           |
| `Add`       | Append codecs to the chain                      |
| `Run`       | Execute the chain for a field                   |
| `HasCodecs` | Report whether any codecs are registered        |

## Built-in codecs

| Subpackage  | Conversion                                          |
|-------------|-----------------------------------------------------|
| `pbwrap`    | `wrapperspb.*` <-> Go primitives                    |
| `tspb`      | `timestamppb.Timestamp` <-> `time.Time` / `int64`   |
| `unixtime`  | `time.Time` <-> `int64` Unix timestamp               |
| `jsonpb`    | `json.RawMessage` <-> `structpb.Struct`              |
| `oneof`     | Struct <-> protobuf oneof interface                  |
| `mapslice`  | Map keys/values -> slice                             |
