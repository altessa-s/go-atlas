# maps

```go
import "github.com/altessa-s/go-atlas/core/collections/maps"
```

Package `maps` provides generic utilities for map transformation, filtering, conversion, pooling, and weak references. All pure functions return new 
maps without modifying inputs.

## Functions

| Function                 | Description                                          |
|--------------------------|------------------------------------------------------|
| `Merge`                  | Combine two maps; source keys take precedence        |
| `Swap`                   | Transpose keys and values                            |
| `FilterMap`              | Return entries matching a predicate                  |
| `ConvertMap`             | Transform keys and values via a mapping function     |
| `FromSlice`              | Build a map from a slice using a key extractor       |
| `FromSliceWith`          | Build a map from a slice using a key-value extractor |
| `ToKeyValueSlice`        | Flatten a map to `[]any{k1, v1, k2, v2, ...}`        |
| `FromFlatMap`            | Expand dot-separated keys into nested maps           |
| `FromFlatMapWithHandler` | Same as `FromFlatMap` with a conflict callback       |
| `ToFlatMap`              | Flatten nested maps into dot-separated keys          |

## Iterators (Go 1.23+)

Lazy `iter.Seq` / `iter.Seq2` iterators for zero-allocation pipelines. Use `slices.Collect()` to materialize.

| Iterator | Description                        |
|----------|------------------------------------|
| `Keys`   | Yield all keys                     |
| `Values` | Yield all values                   |
| `Filter` | Yield entries matching a predicate |
| `Map`    | Yield transformed key-value pairs  |

## Pool

`Pool[K, V]` is a generic, concurrency-safe `sync.Pool` wrapper for reusing map allocations. Maps exceeding 1024 entries are discarded on return.

```go
p := maps.NewPool[string, int](128)
m := p.Get()
// use *m ...
p.Put(m)
```

## WeakMap

`WeakMap[K, V]` holds weak references to values. Entries are automatically removed when the GC reclaims the value via `runtime.AddCleanup` — no periodic sweeps needed.

```go
wm := maps.NewWeakMap[string, MyService]()
wm.Set("svc", svc)

if v, ok := wm.Get("svc"); ok {
    // v is still alive
}
```

## WeakRef

`WeakRef[T]` is a thin wrapper around `weak.Pointer` for holding a single weak reference.

```go
ref := maps.MakeWeakRef(obj)
if ref.IsAlive() {
    val := ref.Value()
}
```
