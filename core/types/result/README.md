# result

```go
import "github.com/altessa-s/go-atlas/core/types/result"
```

Package `result` provides a generic `Result[T]` value that holds either a value of type `T` or an `error`. It is meant for places where Go's idiomatic `(T, error)` tuple is awkward to express — channel elements, slice values, map values — not as a general replacement for `(T, error)` returns.

The zero value of `Result[T]` is a valid `Ok` of the zero value of `T`. All constructors and methods are pure and safe for concurrent use.

## Functions

| Function   | Description                                                              |
|------------|--------------------------------------------------------------------------|
| `Ok[T]`    | `T` -> `Result[T]` (success carrying `v`)                                |
| `Err[T]`   | `error` -> `Result[T]` (failure; passing `nil` yields the zero `Ok`)     |
| `Of[T]`    | `(T, error)` -> `Result[T]` (canonical bridge from existing call sites)  |

## Methods

| Method               | Description                                                          |
|----------------------|----------------------------------------------------------------------|
| `Get() (T, error)`   | Underlying pair; the bridge back to idiomatic Go control flow        |
| `Value() T`          | Contained value (zero value of `T` if `Result` carries an error)     |
| `Err() error`        | Contained error, or `nil` if `Result` is `Ok`                        |
| `IsOk() bool`        | Reports whether `Err()` is `nil`                                     |
| `IsErr() bool`       | Reports whether `Err()` is non-nil                                   |
| `OrDefault(def T) T` | Contained value if `Ok`, otherwise `def`                             |
| `OrElse(fn) T`       | Contained value if `Ok`, otherwise `fn(err)` (only invoked on error) |

## When to use

- Channels of independent results: `chan Result[T]` instead of an ad-hoc struct.
- Slices or maps where each element may independently succeed or fail and the consumer needs both successes and failures.
- Buffering a `(T, error)` pair across an asynchronous boundary where you want to defer the `if err != nil` decision to a single place.

## When NOT to use

- **Ordinary synchronous calls.** Keep returning `(T, error)` and use `if err != nil`. Wrapping return values in `Result[T]` adds noise without removing any.
- **Panic on error.** Use the existing helper instead of adding an `Unwrap`-style method:

  ```go
  v := panics.MustResult(r.Get())
  ```

  See [`core/runtime/panics`](../../runtime/panics/README.md).
- **Asynchronous fan-out with aggregated errors.** Prefer [`core/runtime/concurrency.ProcessCollect`](../../runtime/concurrency/README.md) over a hand-rolled `chan Result[T]`.
- **Functional pipelines (`Map`, `AndThen`, `Then`, …).** In Go they read as nested closures rather than terse pipelines, so plain `if err != nil` stays shorter and clearer. Convert back with `Get` and use ordinary control flow.

## Usage

```go
results := make(chan result.Result[Page])
go func() {
    defer close(results)
    for _, url := range urls {
        results <- result.Of(fetch(url))
    }
}()

for r := range results {
    page, err := r.Get()
    if err != nil {
        log.Warn("fetch", "err", err)
        continue
    }
    process(page)
}
```
