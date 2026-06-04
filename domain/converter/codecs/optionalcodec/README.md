# optionalcodec

```go
import "github.com/altessa-s/go-atlas/domain/converter/codecs/optionalcodec"
```

Package `optionalcodec` provides a [`converter`](../../README.md) codec that bridges [`optional.Optional[T]`](../../../../core/types/optional/README.md)
fields and matching pointer or value counterparts during struct-to-struct conversion.

Use it when your domain entity carries `Optional[T]` for fields where the Mongo model (or any other DTO) still uses `*T` or the bare `T`. The codec
turns the asymmetric mapping into a no-boilerplate single-line registration.

## Supported shapes

| Source         | Destination    | Behavior                                                               |
|----------------|----------------|------------------------------------------------------------------------|
| `Optional[T]`  | `Optional[T]`  | direct copy                                                            |
| `Optional[T]`  | `*T`           | `None` → `nil`; `Some(v)` → `&v`                                       |
| `Optional[T]`  | `T`            | `None` → zero `T`; `Some(v)` → `v`                                     |
| `*T`           | `Optional[T]`  | `nil` → `None`; non-nil → `Some(*p)`                                   |
| `T`            | `Optional[T]`  | `v` → `Some(v)`                                                        |
| anything else  | anything else  | delegated back to the codec chain (the codec is a no-op for that pair) |

## Usage

Register the codec when constructing the `data/mongo` client:

```go
m, err := mongo.New("mydb",
    mongo.WithConverterOptions(
        converter.WithCodecs(optionalcodec.Codec),
    ),
)
```

Or wire it directly into a converter:

```go
conv := converter.NewShared[ModelT, EntityT](
    converter.WithCodecs(optionalcodec.Codec),
)
```

## How it works

The codec uses the narrow reflection escape hatch published by `core/types/optional` (`IsOptionalType`, `InnerType`, `GetReflect`, `SomeReflect`,
`NoneReflect`). It never reads or writes `Optional`'s private layout directly; the trade-off is one extra indirection per Optional field but a
strictly contained dependency on the type's internals.

The codec is safe for concurrent use and allocates at most one `reflect.Value` per call — only when an `Optional` has to be constructed or
extracted.
