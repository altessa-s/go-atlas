# codec

```go
import convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
```

Package `convcodec` provides the codec interface and chain execution engine for the converter. Individual codec implementations live in subpackages.

A `Codec` either handles the conversion and returns, or delegates to `next`. Codecs execute in registration order inside a `Set`.

`CodecE` receives the full source field path and returns an error. Register it
with `AddError`; `RunError` runs error codecs, then legacy codecs, then the
error-returning terminal handler. `HasErrorCodecs` reports whether this path is
needed. When registered with the converter, errors returned by `CodecE`
propagate unchanged through `converter.ConvertE`, preserving error identity.
The existing `Codec` and `Run` APIs are unchanged.

## Set

| Method      | Description                                     |
|-------------|-------------------------------------------------|
| `NewCodecsSet` | Create a `Set` with initial codecs           |
| `Add`       | Append codecs to the chain                      |
| `Run`       | Execute the chain for a field                   |
| `HasCodecs` | Report whether any codecs are registered        |
| `AddError` | Append error codecs to the chain                |
| `HasErrorCodecs` | Report whether error codecs are registered |
| `RunError` | Run error codecs, then legacy codecs, then the error-returning terminal handler |

## Built-in codecs

| Subpackage  | Conversion                                          |
|-------------|-----------------------------------------------------|
| `pbwrap`    | `wrapperspb.*` <-> Go primitives                    |
| `tspb`      | `timestamppb.Timestamp` <-> `time.Time` / `int64`   |
| `durpb`     | `durationpb.Duration` <-> `time.Duration`           |
| `unixtime`  | `time.Time` <-> `int64` Unix timestamp               |
| `jsonpb`    | `json.RawMessage` <-> `structpb.Struct`              |
| `oneof`     | Struct <-> protobuf oneof interface                  |
| `mapslice`  | Map keys/values -> slice                             |
