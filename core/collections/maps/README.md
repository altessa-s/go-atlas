# maps

```go
import "github.com/altessa-s/go-atlas/core/collections/maps"
```

Package `maps` provides generic utilities for map transformation, filtering, conversion, pooling, and weak references. All pure functions
return new maps without modifying inputs; nil maps are treated as empty and never cause panics.

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
| `MergeWith`              | Combine two maps with a custom conflict resolver      |
| `MergeAll`               | Overlay N maps in order; last layer wins              |
| `MergeDeep`              | Recursively merge nested `map[string]any` structures  |
| `FromFlatMap`            | Expand dot-separated keys into nested maps            |
| `FromFlatMapWithHandler` | Same as `FromFlatMap` with a conflict callback        |
| `ToFlatMap`              | Flatten nested maps into dot-separated keys           |

## Iterators

Lazy `iter.Seq` / `iter.Seq2` iterators for zero-allocation pipelines. Use `slices.Collect()` to materialize results into concrete map values.

| Iterator | Description                        |
|----------|------------------------------------|
| `Keys`   | Yield all keys                     |
| `Values` | Yield all values                   |
| `Filter` | Yield entries matching a predicate |
| `Map`    | Yield transformed key-value pairs  |

## Pool

`Pool[K, V]` is a generic, concurrency-safe `sync.Pool` wrapper for reusing map allocations. Maps exceeding 1024 entries are discarded on return.

## WeakMap

`WeakMap[K, V]` holds weak references to values. Entries are automatically removed when the GC reclaims the value via `runtime.AddCleanup`
— no periodic sweeps, manual eviction, or background goroutines needed. Safe for concurrent access.

## WeakRef

`WeakRef[T]` is a thin wrapper around `weak.Pointer` for holding a single weak reference. Use `IsAlive` to check and `Value` to retrieve.

## ImmutableMap

`ImmutableMap[K, V]` is a read-only hash map that cannot be modified after construction. Build once from a `map[K]V` or parallel key/value slices,
then read concurrently without locks or defensive copies. Uses a Swiss-table layout with flat arrays for lower memory overhead (~1.5–2× savings at
scale) and reduced GC pressure compared to a standard Go map.

```go
src := map[string]int{"a": 1, "b": 2, "c": 3}
m := maps.NewImmutableMap(src)
v, ok := m.Get("a") // 1, true
```

| Method                   | Description                                        |
|--------------------------|----------------------------------------------------|
| `NewImmutableMap`        | Build from a standard Go map                       |
| `NewImmutableMapFromEntries` | Build from parallel key and value slices       |
| `Get`                    | Lookup by key; returns value and presence flag      |
| `Contains`               | Check key presence                                  |
| `Len`                    | Number of entries                                   |
| `All`                    | Iterator over all key-value pairs (`iter.Seq2`)     |
| `Keys`                   | Iterator over keys (`iter.Seq`)                     |
| `Values`                 | Iterator over values (`iter.Seq`)                   |
