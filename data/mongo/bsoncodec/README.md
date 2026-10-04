# bsoncodec

```go
import "github.com/altessa-s/go-atlas/data/mongo/bsoncodec"
```

MongoDB registry for [`optional.Optional[T]`](../../../core/types/optional), keeping database dependencies out of universal value types. Existing driver
codecs and custom inner-type codecs continue to work.

## API

| Function | Behavior |
|----------|----------|
| `NewRegistry()` | Driver defaults plus codecs for every Optional instantiation; configure before concurrent use |

## Usage

```go
reg := bsoncodec.NewRegistry()
client, err := mongo.Connect(options.Client().SetRegistry(reg))
```

For standalone BSON, set the same registry on both `bson.NewEncoder(bson.NewDocumentWriter(writer))` and
`bson.NewDecoder(bson.NewDocumentReader(reader))` with `SetRegistry(reg)`. `bson.Marshal` and `bson.Unmarshal` use the driver's default registry and
cannot serialize Optional without configuration; they return `optional.ErrBSONCodecRequired` for present Optional fields. None with `omitempty` is
omitted without invoking the codec.

## Semantics

| Value | BSON |
|-------|------|
| `Some(v)` | Encoding of `v`, using this registry |
| `None` | Null; omitted when the field has `omitempty` |
| Null on decode | None |
| Missing field | Existing destination field unchanged; None on a fresh destination |
| Invalid inner value | Error; the Optional destination remains unchanged |
| `Some(nil)` | Null; decodes as None because BSON has no presence bit |

The [`data/mongo`](../README.md) client constructor installs this registry unless explicitly overridden. Injected clients and custom registries remain
the caller's responsibility. RedactedString's fixed redaction hook also works with this registry and with the driver's default registry.
