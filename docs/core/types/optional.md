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

| Symbol group | Members                                |
|--------------|----------------------------------------|
| Constructors | `Some`, `None`, `Of`                   |
| Accessors    | `Get`, `Value`, `IsSome`, `IsNone`     |
| Fallbacks    | `OrDefault`, `OrElse`                  |

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
- `core/types/ptr` — pointer constructors and safe dereference for the cases where `*T` is the right encoding.
- `core/types/optional/README.md` — package-level reference next to the source.
