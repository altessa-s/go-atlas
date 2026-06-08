# Optional

Generic value-or-absent type that makes the "value may be missing" intent explicit at the type level. Use it where Go's idiomatic `(T, bool)`
pair or `*T` is awkward — struct fields, channel elements, slice values, map values.

```go
import "github.com/altessa-s/go-atlas/core/types/optional"
```

`Optional[T]` is a value type built from two fields (`value T`, `present bool`). Construction never allocates and the type is comparable when
`T` is comparable. The zero value is a valid `None`. Unlike a `*T`, an `Optional[T]` distinguishes `Some(zero)` from `None` without forcing a
heap allocation per `Some`.

> **Not a Go-idiom replacement.** Keep `value, ok := m[k]` and `value, ok := <-ch` as they are. Reach for `Optional[T]` when the pair cannot
> ride along — when the value lives in a struct field, in a channel element, or in a slice.

---

## API overview

| Symbol group  | Members                                                                  |
|---------------|--------------------------------------------------------------------------|
| Constructors  | `Some`, `None`, `Of`, `FromPtr`                                          |
| Accessors     | `Get`, `Value`, `IsSome`, `IsNone`, `IsZero`                             |
| Fallbacks     | `OrDefault`, `OrElse`                                                    |
| Conversions   | `ToPtr`                                                                  |
| Serialization | `MarshalBSONValue`, `UnmarshalBSONValue`, `MarshalJSON`, `UnmarshalJSON` |
| Reflection    | `IsOptionalType`, `InnerType`, `GetReflect`, `SomeReflect`, `NoneReflect`|

---

## Constructors

### Some

`Some[T](v T) Optional[T]` wraps a present value. `v` may itself be the zero value of `T`; the resulting `Optional` still reports `IsSome() == true`:

```go
opt := optional.Some("")     // present, equal to ""
opt.IsSome()                  // true
```

### None

`None[T]() Optional[T]` is an empty `Optional`. Equivalent to the zero value of the type:

```go
var a optional.Optional[int]
b := optional.None[int]()
a == b                        // true
```

### Of

`Of[T](v T, ok bool) Optional[T]` is the canonical bridge from the idiomatic `(value, ok)` lookup. When `ok` is false `v` is discarded and the
result carries the zero value of `T`:

```go
opt := optional.Of(m[key])   // Some(v) if found, None otherwise
```

### FromPtr

`FromPtr[T](ptr *T) Optional[T]` builds `None` from a nil pointer and `Some(*ptr)` from a non-nil pointer. Useful at the boundary with legacy
APIs that still return `*T`:

```go
opt := optional.FromPtr(legacyAPI())
```

`ToPtr[T](opt Optional[T]) *T` is the reverse direction (`None → nil`, `Some(v) → &v`) — listed under **Conversions** below.

---

## Accessors

### Get

`Get() (T, bool)` is the primary accessor and the bridge **back** to ordinary Go control flow:

```go
if v, ok := opt.Get(); ok {
    use(v)
}
```

### Value, IsSome, IsNone

Direct accessors for code that wants only one side of the pair:

| Method          | Returns                                                                                                                       |
|-----------------|-------------------------------------------------------------------------------------------------------------------------------|
| `Value() T`     | The contained value as-is. Returns the zero value of `T` when the `Optional` is `None` — use `Get` if you must distinguish a real `Some(zero)` from `None`. |
| `IsSome() bool` | The `Optional` carries a value.                                                                                               |
| `IsNone() bool` | The `Optional` is empty.                                                                                                      |
| `IsZero() bool` | Synonym for `IsNone`. Lets the BSON encoder treat `Optional` as a zero value under `bson:",omitempty"` (see **Serialization**). |

---

## Fallbacks

### OrDefault

`OrDefault(def T) T` returns the contained value when `Some`, otherwise `def`:

```go
host := optional.Of(m["host"]).OrDefault("localhost")
```

### OrElse

`OrElse(fn func() T) T` returns the contained value when `Some`, otherwise the result of calling `fn()`. `fn` is invoked only on the `None`
path, so callers can defer expensive defaults:

```go
v := opt.OrElse(func() int {
    return loadDefaultFromDisk()
})
```

---

## Conversions

### ToPtr

`ToPtr[T](opt Optional[T]) *T` returns `nil` when `opt` is `None` and a pointer to the contained value when `opt` is `Some`. It is the
counterpart to `FromPtr` and is the right escape hatch when handing the value to a legacy API that expects `*T`:

```go
opt := optional.Some("hi")
legacy.SetField(optional.ToPtr(opt))   // legacy.SetField(*string)
```

Note that `ToPtr` allocates: the returned pointer is the address of an internal copy of the value, not of the storage inside the `Optional`.

---

## Serialization

`Optional[T]` implements `bson.ValueMarshaler` / `bson.ValueUnmarshaler` and `json.Marshaler` / `json.Unmarshaler`, so it works out of the box
with `go.mongodb.org/mongo-driver/v2` and `encoding/json`. `Some(v)` is encoded as the underlying `v`; `None` is encoded as BSON `null` or
JSON `null`. `Some(zero T)` is preserved through a round-trip — it does not collapse to `None`.

| Direction        | `Some(v)`           | `None`              | Notes                                                                                  |
|------------------|---------------------|---------------------|----------------------------------------------------------------------------------------|
| BSON marshal     | underlying BSON `v` | BSON `null`         | Combined with `IsZero`, `bson:",omitempty"` omits `None` fields entirely from the wire |
| BSON unmarshal   | `Some(v)`           | `None`              | BSON `null` or an absent field both decode to `None`                                   |
| JSON marshal     | `json.Marshal(v)`   | literal `null`      | `encoding/json` does not consult `IsZero`; a `None` field always emits `null`          |
| JSON unmarshal   | `Some(v)`           | `None`              | JSON `null` or an absent field both decode to `None`                                   |

```go
type Doc struct {
    DeletedAt optional.Optional[time.Time] `bson:"deleted_at,omitempty" json:"deleted_at"`
    Note      optional.Optional[string]    `bson:"note,omitempty"      json:"note"`
}

raw, _ := bson.Marshal(Doc{DeletedAt: optional.None[time.Time]()})
// raw does not contain "deleted_at" at all (IsZero + omitempty).

raw, _ = bson.Marshal(Doc{Note: optional.Some("")})
// raw DOES contain "note": "" — Some(zero) is not collapsed to None.
```

When the matching Mongo model still uses `*T` (asymmetric `Optional ↔ *T` mapping), pair this with the codec from
[`domain/converter/codecs/optionalcodec`](../../../domain/converter/codecs/optionalcodec/README.md) and pass it through
`data/mongo.WithConverterOptions`:

```go
m, err := mongo.New("mydb",
    mongo.WithConverterOptions(
        converter.WithCodecs(optionalcodec.Codec),
    ),
)
```

Because the marshaller methods must live on the type itself, `core/types/optional` is the only `core/*` package with a non-stdlib dependency
(`go.mongodb.org/mongo-driver/v2/bson`). The trade-off is intentional: it makes `Optional` a first-class Mongo field type without forcing a
wrapper type at every call site.

---

## Reflection escape hatch

A small set of reflection helpers ships alongside `Optional[T]` for codec authors and serializers that need to detect, read, and construct
`Optional[T]` values without compile-time knowledge of `T`. Ordinary user code should never reach for these — use the type system instead.

| Function                                       | Purpose                                                                            |
|------------------------------------------------|------------------------------------------------------------------------------------|
| `IsOptionalType(t reflect.Type) bool`          | Reports whether `t` is an `Optional[T]` instantiation from this package            |
| `InnerType(optType reflect.Type) reflect.Type` | Returns the type parameter `T` of an `Optional[T]`; panics on non-Optional         |
| `GetReflect(opt reflect.Value) (reflect.Value, bool)` | Extracts the `(value, present)` pair from an Optional reflect value         |
| `SomeReflect(optType, value reflect.Value) reflect.Value` | Builds `Some(value)` as a `reflect.Value` of `optType`                  |
| `NoneReflect(optType reflect.Type) reflect.Value` | Returns a `None` `reflect.Value` of `optType`                                   |

Internally these use `reflect.NewAt` + `unsafe.Pointer` to alias the unexported `value`/`present` fields — the same technique
`encoding/json` and `encoding/gob` use to populate user structs. They live inside the same package as `Optional[T]` so the unsafe surface
stays in one well-defined place.

The canonical consumer is [`domain/converter/codecs/optionalcodec`](../../../domain/converter/codecs/optionalcodec/README.md), which bridges
`Optional[T] ↔ *T`, `Optional[T] ↔ T`, and `Optional[T] ↔ Optional[T]` during struct-to-struct conversion.

---

## When to use

- **Struct fields where the zero value of `T` is itself a valid value** and you need to distinguish it from "not set" — e.g.
  `Optional[string]` to tell `Some("")` from `None`, or `Optional[int]` to tell `Some(0)` from `None`.
- **Channel/slice/map element types** where `nil` would be ambiguous, where `T` is not nillable, or where you want the absent case to be a
  distinct value rather than a separate flag.
- **Function parameters** whose presence carries semantic weight you want the type signature to advertise.

## When NOT to use

- **Ordinary lookups.** Keep `value, ok := m[k]` and `value, ok := <-ch` — wrapping each in `Optional` adds noise.
- **Pointer fields where `nil` already means "absent".** `*T` is fine when callers cannot legitimately construct `Some(zero)`.
- **"Value or error" cases.** Use [`Result[T]`](result.md), not `Optional[T]` plus a side-channel error.
- **Functional pipelines (`Map`, `AndThen`, `Then`, …).** In Go they read as nested closures rather than terse pipelines, so plain `if ok`
  stays shorter and clearer. Convert back with `Get`.

---

## Performance notes

- **Zero-allocation construction.** Internally `Optional[T]` is `(value T, present bool)`, never `*T`. Benchmarks show `0 B/op` and
  `0 allocs/op` on every method.
- **Comparable when `T` is comparable.** `optional.Some(1) == optional.Some(1)` and `optional.None[int]() == optional.None[int]()`.
  `optional.Some(0) != optional.None[int]()` — exactly the property `*T` cannot give you without an extra heap object.
- **Zero value is a valid `None`.** `var o Optional[int]` is `None`. Safe to embed in struct fields without an explicit constructor.

### Optional vs *T

| Property                                | `*T`                            | `Optional[T]`                                  |
|-----------------------------------------|---------------------------------|------------------------------------------------|
| Construction allocates                  | Yes — heap escape on `&v`       | No — `(value, present)` lives by value         |
| Distinguishes `Some(zero)` from `None`  | No — `&zeroT` and `nil` overlap in intent | Yes — `present` is independent of `value` |
| Comparable                              | By pointer identity, not value  | By value, when `T` is comparable               |
| Type signature signals optionality      | Only "may be nil"               | "Optional" is right there in the type          |

Pointer-based optionality is the right tool when callers already think of the field as a borrow ("here is a thing or nothing") and the value
lives elsewhere; `Optional[T]` is the right tool when the value is owned by the field itself and zero is a meaningful state. See
[`core/types/ptr`](../../../core/types/ptr/README.md) for the pointer-based primitives.

---

## See also

- [result.md](result.md) — sibling type for "value or error".
- [redacted.md](redacted.md) — sibling type for credential and sensitive-string fields that redact themselves in fmt, slog, JSON, YAML, and BSON.
- `core/types/ptr` — pointer constructors and safe dereference for the cases where `*T` is the right encoding.
- `core/types/optional/README.md` — package-level reference next to the source.
- [`domain/converter/codecs/optionalcodec`](../../../domain/converter/codecs/optionalcodec/README.md) — converter codec bridging
  `Optional[T] ↔ *T` / `T` / `Optional[T]` for struct-to-struct conversion (used by `data/mongo.GetEntity` / `GetEntities`
  via `mongo.WithConverterOptions`).
