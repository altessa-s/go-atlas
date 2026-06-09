# optionalcodec

```go
import "github.com/altessa-s/go-atlas/domain/converter/codecs/optionalcodec"
```

Package `optionalcodec` provides a [`converter`](../../README.md) codec that bridges [`optional.Optional[T]`](../../../../core/types/optional/README.md)
fields and matching pointer or value counterparts during struct-to-struct conversion.

Use it when your domain entity carries `Optional[T]` for fields where the Mongo model (or any other DTO) still uses `*T` or the bare `T`. The codec
turns the asymmetric mapping into a no-boilerplate single-line registration.

## Supported shapes

| Source         | Destination    | Behavior                                                                                          |
|----------------|----------------|---------------------------------------------------------------------------------------------------|
| `Optional[T]`  | `Optional[T]`  | direct copy (same inner type)                                                                     |
| `Optional[T]`  | `*T`           | `None` → `nil`; `Some(v)` → `&v`                                                                  |
| `Optional[T]`  | `T`            | `None` → zero `T`; `Some(v)` → `v`                                                                |
| `*T`           | `Optional[T]`  | `nil` → `None`; non-nil → `Some(*p)`                                                              |
| `T`            | `Optional[T]`  | `v` → `Some(v)`                                                                                   |
| `Optional[T]`  | `W` (any)      | unwrap and delegate `T → W` to the chain — see [Composition](#composition-with-downstream-codecs) |
| `W` (any)      | `Optional[T]`  | delegate `W → T` to the chain, then wrap — see [Composition](#composition-with-downstream-codecs) |
| anything else  | anything else  | delegated back to the codec chain (the codec is a no-op for that pair)                            |

`Optional[A] ↔ Optional[B]` with different inner types is **not** supported and falls through to the chain — register a custom codec that handles
the specific pair if you need that bridge.

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

## Composition with downstream codecs

When one side is `Optional[T]` and the other is some type `W` that this codec does not bridge directly (e.g. `*timestamppb.Timestamp`,
`*durationpb.Duration`), the codec unwraps / wraps the `Optional` and delegates the inner `T ↔ W` conversion to the rest of the chain. This lets the
codec compose with type-specific bridges such as [`tspb`](../../codec/tspb/README.md) and [`durpb`](../../codec/durpb/README.md) without requiring a
dedicated `Optional`-aware variant of each one.

**Ordering matters:** register `optionalcodec.Codec` *before* the codec that handles the inner type. The chain runs codecs in registration order, so
the `Optional` wrapper must be stripped first for the downstream codec to see the inner value.

**Presence rule on `W → Optional[T]`:** after the chain materializes the inner `T`, the codec applies `reflect.Value.IsZero` (the struct-zero test,
not the type's own `IsZero` method). A struct-zero inner collapses to `None`, anything else becomes `Some`. For `time.Time` this is unambiguous —
`tspb.WithIgnoreZero` skips a `nil` / epoch `*Timestamp` so the inner `time.Time` is never written and stays the pristine zero. For `time.Duration`
this is the observable consequence of the same rule: a non-nil `*durationpb.Duration{Seconds: 0}` collapses to `None`, matching the
`None ↔ nil/zero` convention used by the direct shapes.

### Example: proto boundary

Typical wiring at the gRPC handler boundary, mapping domain entities with `Optional[T]` fields to/from generated `*pb` types:

```go
import (
    "github.com/altessa-s/go-atlas/domain/converter"
    "github.com/altessa-s/go-atlas/domain/converter/codec/durpb"
    "github.com/altessa-s/go-atlas/domain/converter/codec/tspb"
    "github.com/altessa-s/go-atlas/domain/converter/codecs/optionalcodec"
)

// Domain side: Optional[T] makes presence explicit.
type Project struct {
    DeleteTime optional.Optional[time.Time]
    TTL        optional.Optional[time.Duration]
}

// Wire side: generated proto types use *Timestamp / *Duration.
type ProjectPB struct {
    DeleteTime *timestamppb.Timestamp
    TTL        *durationpb.Duration
}

opts := []converter.Option{
    converter.WithCodecs(
        optionalcodec.Codec,             // strips/wraps Optional first
        tspb.New(tspb.WithIgnoreZero()), // bridges time.Time ↔ *Timestamp
        durpb.New(durpb.WithIgnoreZero()),
    ),
    converter.WithIgnoreZeroValues(),
}

var pb ProjectPB
converter.Convert(&Project{DeleteTime: optional.Some(now)}, &pb, opts...)
// pb.DeleteTime is a non-nil *Timestamp; pb.TTL is nil (None → nil).
```

The reverse direction works with the same chain (omit `WithIgnoreZeroValues()` when you want the converter to write zero values explicitly).

## How it works

The codec uses the narrow reflection escape hatch published by `core/types/optional` (`IsOptionalType`, `InnerType`, `GetReflect`, `SomeReflect`,
`NoneReflect`). It never reads or writes `Optional`'s private layout directly; the trade-off is one extra indirection per Optional field but a
strictly contained dependency on the type's internals.

The codec is safe for concurrent use and allocation-light: a direct shape builds at most one `reflect.Value` (only when an `Optional` has to be
constructed or extracted); the composition path builds at most two (the materialized inner value plus the wrapping `Optional`).
