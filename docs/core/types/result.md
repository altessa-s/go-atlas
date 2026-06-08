# Result

Generic value-or-error tagged union for cases where Go's idiomatic `(T, error)` tuple is awkward to express — channel elements, slice values,
map values. Not a replacement for `(T, error)` returns.

```go
import "github.com/altessa-s/go-atlas/core/types/result"
```

`Result[T]` is a value type built from two fields (`value T`, `err error`). Construction never allocates and the type is comparable when both
`T` and the underlying error are comparable. The zero value is a valid `Ok` of the zero value of `T`.

> **Not a Go-style error replacement.** Functions should keep returning `(T, error)`; reach for `Result[T]` only when a tuple cannot ride along
> (channel element, slice item, struct field).

---

## API overview

| Symbol group | Members                              |
|--------------|--------------------------------------|
| Constructors | `Ok`, `Err`, `Of`                    |
| Accessors    | `Get`, `Value`, `Err`, `IsOk`, `IsErr` |
| Fallbacks    | `OrDefault`, `OrElse`                |

---

## Constructors

### Ok

`Ok[T](v T) Result[T]` wraps a successful value.

```go
r := result.Ok(42)
r.IsOk() // true
```

### Err

`Err[T](err error) Result[T]` wraps an error. Passing `nil` produces a `Result` indistinguishable from `Ok` of the zero value, so callers that
mean "failure" must pass a non-nil error.

```go
r := result.Err[int](io.ErrUnexpectedEOF)
r.IsErr() // true
```

### Of

`Of[T](v T, err error) Result[T]` is the canonical bridge from the idiomatic `(T, error)` pair. Most call sites that build a `Result` use this:

```go
results <- result.Of(os.ReadFile(path))
```

---

## Accessors

### Get

`Get() (T, error)` is the primary accessor and the bridge **back** to ordinary Go control flow. It returns the underlying pair so callers can
use `if err != nil` as usual:

```go
for r := range results {
    page, err := r.Get()
    if err != nil {
        log.Warn("fetch", "err", err)
        continue
    }
    process(page)
}
```

### Value, Err, IsOk, IsErr

Direct accessors for cases that want only one side of the pair:

| Method         | Returns                                                                                                                               |
|----------------|---------------------------------------------------------------------------------------------------------------------------------------|
| `Value() T`    | The contained value as-is. Returns the zero value of `T` when the `Result` carries an error — use `Get` if you must distinguish a real `Ok(zero)` from a default-on-error. |
| `Err() error`  | The contained error, or `nil` if `Ok`.                                                                                                |
| `IsOk() bool`  | `Err()` is `nil`.                                                                                                                     |
| `IsErr() bool` | `Err()` is non-nil.                                                                                                                   |

---

## Fallbacks

### OrDefault

`OrDefault(def T) T` returns the contained value when `Ok`, otherwise `def`. Useful when the default is cheap (a constant or already-bound variable):

```go
host := result.Of(os.LookupEnv("HOST")).OrDefault("localhost")
```

### OrElse

`OrElse(fn func(error) T) T` returns the contained value when `Ok`, otherwise the result of calling `fn(err)`. `fn` is invoked only on the
error path, so callers can defer expensive work or log the error before falling back:

```go
v := r.OrElse(func(err error) int {
    log.Warn("falling back", "err", err)
    return -1
})
```

---

## When to use

- **Channels** of independent results: `chan Result[T]` instead of an ad-hoc struct with `value` and `err` fields.
- **Slices or maps** where each element may independently succeed or fail and the consumer wants both successes and failures.
- **Buffering a `(T, error)` pair across an asynchronous boundary** where you want to defer the `if err != nil` decision to a single place.

## When NOT to use

- **Ordinary synchronous calls.** Keep returning `(T, error)` and use `if err != nil`. Wrapping return values in `Result[T]` adds noise without
  removing any.
- **Panic on error.** Use the existing helper instead of adding an `Unwrap`-style method:

  ```go
  v := panics.MustResult(r.Get())
  ```

  See [`core/runtime/panics`](../runtime/README.md).
- **Asynchronous fan-out with aggregated errors.** Prefer [`core/runtime/concurrency.ProcessCollect`](../runtime/concurrency.md) over a
  hand-rolled `chan Result[T]`.
- **Functional pipelines (`Map`, `AndThen`, `Then`, …).** In Go they read as nested closures rather than terse pipelines, so plain
  `if err != nil` stays shorter and clearer. Convert back with `Get`.

---

## Performance notes

- **Zero-allocation construction.** `Ok`, `Err`, and `Of` build the value entirely on the stack; benchmarks show `0 B/op` and `0 allocs/op` on
  every method.
- **Comparable when underlying types are comparable.** Two `Result[T]` values compare equal under `==` iff both their value and error fields
  compare equal. Useful for table-driven tests; reach for explicit accessors when one side is not comparable (e.g. `Result[func()]`).
- **Zero value is a valid `Ok`.** `var r Result[int]` is `Ok(0)`. Safe to embed in struct fields without an explicit constructor.

---

## See also

- [optional.md](optional.md) — sibling type for "value or absent" without an error channel.
- [redacted.md](redacted.md) — sibling type for credential and sensitive-string fields that redact themselves in fmt, slog, JSON, YAML, and BSON.
- [`core/runtime/panics`](../runtime/README.md) — `MustResult[T]` for panic-on-error semantics.
- [../runtime/concurrency.md](../runtime/concurrency.md) — `Process`/`ProcessCollect` for fan-out with aggregated errors.
- `core/types/result/README.md` — package-level reference next to the source.
