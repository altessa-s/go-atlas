# optional

```go
import "github.com/altessa-s/go-atlas/core/types/optional"
```

Package `optional` provides a generic `Optional[T]` value that holds either a value of type `T` or nothing. It makes the "value may be absent"
intent explicit at the type level, for places where Go's idiomatic `(T, bool)` pair or `*T` is awkward — struct fields, channel elements, slice
values, map values.

Internally `Optional[T]` carries `(value, present)` by value: `Some(v)` does **not** allocate, the type is comparable when `T` is comparable, and
the zero value of `Optional[T]` is a valid `None`.

## Functions

| Function      | Description                                                              |
|---------------|--------------------------------------------------------------------------|
| `Some[T]`     | `T` -> `Optional[T]` (carries `v`)                                       |
| `None[T]`     | `()` -> `Optional[T]` (empty)                                            |
| `Of[T]`       | `(T, bool)` -> `Optional[T]` (canonical bridge from `value, ok` lookups) |
| `FromPtr[T]`  | `*T` -> `Optional[T]` (None if nil, Some with dereferenced value)        |
| `ToPtr[T]`    | `Optional[T]` -> `*T` (nil if None, pointer to value if Some)           |

## Methods

| Method                                     | Description                                                          |
|--------------------------------------------|----------------------------------------------------------------------|
| `Get() (T, bool)`                          | Underlying pair; the bridge back to idiomatic Go control flow        |
| `Value() T`                                | Contained value (zero value of `T` if `Optional` is `None`)          |
| `IsSome() bool`                            | Reports whether the `Optional` carries a value                       |
| `IsNone() bool`                            | Reports whether the `Optional` is empty                              |
| `OrDefault(def T) T`                       | Contained value if `Some`, otherwise `def`                           |
| `OrElse(fn) T`                             | Contained value if `Some`, otherwise `fn()` (only invoked on `None`) |
| `IsZero() bool`                            | `true` when `None`; lets `bson:",omitempty"` strip `None` fields     |
| `MarshalBSONValue() (byte, []byte, error)` | Some → underlying BSON value; None → BSON `null`                     |
| `UnmarshalBSONValue(byte, []byte) error`   | BSON `null` → None; otherwise decode into `T` and store as Some      |
| `MarshalJSON() ([]byte, error)`            | Some → `json.Marshal(v)`; None → `null`                              |
| `UnmarshalJSON([]byte) error`              | JSON `null` → None; otherwise decode into `T` and store as Some      |

## Serialization

`Optional[T]` implements `bson.ValueMarshaler` / `bson.ValueUnmarshaler` and `json.Marshaler` / `json.Unmarshaler`, so it works out of the box with
the `go.mongodb.org/mongo-driver/v2` driver and `encoding/json`. `Some(v)` is encoded as `v`, `None` as `null`. Combined with `IsZero`, the
`bson:",omitempty"` tag omits `None` fields entirely from the on-the-wire document.

`Some(zeroT)` is preserved through a round-trip — it does not collapse to `None` — letting callers distinguish "absent" from "present but zero".
Standard `encoding/json` does not consult `IsZero`, so a `None` field marshals as `null`; use `*Optional[T]` when JSON field omission matters.

```go
type Doc struct {
    DeletedAt optional.Optional[time.Time] `bson:"deleted_at,omitempty" json:"deleted_at"`
}

raw, _ := bson.Marshal(Doc{DeletedAt: optional.None[time.Time]()})
// raw does not contain "deleted_at" at all.
```

Because the marshaller methods must live on the type, this is the only `core/*` package with a non-stdlib dependency
(`go.mongodb.org/mongo-driver/v2/bson`). The trade-off is intentional: it makes `Optional` a first-class Mongo field type.

## When to use

- Struct fields where the zero value of `T` is itself a valid value and you need to distinguish it from "not set" — e.g. `Optional[string]` to
  tell `Some("")` from `None`.
- Channel/slice/map element types where `nil` would be ambiguous or where `T` is not nillable.
- Function parameters whose presence carries semantic weight you want the type signature to advertise.

## When NOT to use

- **Ordinary lookups.** Keep the `value, ok := m[k]` and `value, ok := <-ch` idioms — wrapping each in `Optional` adds noise.
- **Pointer fields where `nil` already means "absent".** `*T` is fine when callers cannot construct `Some(zero)` legitimately.
- **"Value or error" cases.** Use [`core/types/result`](../result/README.md), not `Optional[T]` plus a side-channel error.
- **Functional pipelines (`Map`, `AndThen`, `Then`, …).** In Go they read as nested closures rather than terse pipelines, so plain `if ok` stays
  shorter and clearer. Convert back with `Get` and use ordinary control flow.

## Usage

```go
type User struct {
    Name     string
    Nickname optional.Optional[string]
}

u := User{Name: "Ada", Nickname: optional.Some("")}
if nick, ok := u.Nickname.Get(); ok {
    fmt.Println("nickname is set:", nick) // distinguishes Some("") from None
}

// Bridge from a (value, ok) lookup.
opt := optional.Of(m[k])
host := opt.OrDefault("localhost")

// Convert from/to pointers.
var ptr *string
opt = optional.FromPtr(ptr) // None
ptr = optional.ToPtr(optional.Some("hello")) // &"hello"
```
