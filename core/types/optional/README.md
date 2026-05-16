# optional

```go
import "github.com/altessa-s/go-atlas/core/types/optional"
```

Package `optional` provides a generic `Optional[T]` value that holds either a value of type `T` or nothing. It makes the "value may be absent" intent explicit at the type level, for places where Go's idiomatic `(T, bool)` pair or `*T` is awkward — struct fields, channel elements, slice values, map values.

Internally `Optional[T]` carries `(value, present)` by value: `Some(v)` does **not** allocate, the type is comparable when `T` is comparable, and the zero value of `Optional[T]` is a valid `None`.

## Functions

| Function    | Description                                                              |
|-------------|--------------------------------------------------------------------------|
| `Some[T]`   | `T` -> `Optional[T]` (carries `v`)                                       |
| `None[T]`   | `()` -> `Optional[T]` (empty)                                            |
| `Of[T]`     | `(T, bool)` -> `Optional[T]` (canonical bridge from `value, ok` lookups) |

## Methods

| Method               | Description                                                          |
|----------------------|----------------------------------------------------------------------|
| `Get() (T, bool)`    | Underlying pair; the bridge back to idiomatic Go control flow        |
| `Value() T`          | Contained value (zero value of `T` if `Optional` is `None`)          |
| `IsSome() bool`      | Reports whether the `Optional` carries a value                       |
| `IsNone() bool`      | Reports whether the `Optional` is empty                              |
| `OrDefault(def T) T` | Contained value if `Some`, otherwise `def`                           |
| `OrElse(fn) T`       | Contained value if `Some`, otherwise `fn()` (only invoked on `None`) |

## When to use

- Struct fields where the zero value of `T` is itself a valid value and you need to distinguish it from "not set" — e.g. `Optional[string]` to tell `Some("")` from `None`.
- Channel/slice/map element types where `nil` would be ambiguous or where `T` is not nillable.
- Function parameters whose presence carries semantic weight you want the type signature to advertise.

## When NOT to use

- **Ordinary lookups.** Keep the `value, ok := m[k]` and `value, ok := <-ch` idioms — wrapping each in `Optional` adds noise.
- **Pointer fields where `nil` already means "absent".** `*T` is fine when callers cannot construct `Some(zero)` legitimately.
- **"Value or error" cases.** Use [`core/types/result`](../result/README.md), not `Optional[T]` plus a side-channel error.
- **Functional pipelines (`Map`, `AndThen`, `Then`, …).** In Go they read as nested closures rather than terse pipelines, so plain `if ok` stays shorter and clearer. Convert back with `Get` and use ordinary control flow.

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
```
