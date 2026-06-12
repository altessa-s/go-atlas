# slices

```go
import "github.com/altessa-s/go-atlas/core/collections/slices"
```

Package `slices` provides generic utilities for slice transformation, filtering, deduplication, grouping, and pooling. All functions return
new slices without modifying inputs; nil slices are treated as empty and never cause panics.

## Functions

| Function               | Description                                              |
|------------------------|----------------------------------------------------------|
| `Deduplicate`          | Remove duplicates, preserving first-occurrence order     |
| `DeduplicateBy`        | Remove duplicates using a custom key function            |
| `Delete`               | Remove the first occurrence of an element                |
| `To`                   | Transform each element via a mapping function            |
| `ToWithFilter`         | Filter and transform in a single pass                    |
| `ToAny`                | Box a typed slice into `[]any`                           |
| `ToStrings`            | Extract string elements from `[]any`                     |
| `FilterFirst`          | Return the first element matching a predicate            |
| `FilterLast`           | Return the last element matching a predicate             |
| `FilterParallel`       | Filter using multiple goroutines for large slices        |
| `MapParallel`          | Transform using multiple goroutines for large slices     |
| `Reduce`               | Fold a slice into a single value with an accumulator     |
| `GroupBy`              | Partition elements into groups by a key function         |
| `Any`                  | True if at least one element matches a predicate         |
| `All`                  | True if every element matches a predicate                |
| `AppendIf`             | Conditionally append values                              |
| `AppendIfFunc`         | Conditionally append with lazy evaluation                |
| `AppendNonEmpty`       | Append a key-value pair when the value is non-empty      |
| `AppendNonNil`         | Append a value when it is non-nil                        |
| `AppendNonNilErr`      | Append with lazy evaluation and error propagation        |
| `ToList`               | Convert a slice to `container/list.List`                 |
| `FromList`             | Convert a `container/list.List` to a typed slice         |

## Ordering

| Function                 | Description                                    |
|--------------------------|------------------------------------------------|
| `IsStrictlyIncreasing`   | Every pair satisfies `a < b`                   |
| `IsStrictlyDecreasing`   | Every pair satisfies `a > b`                   |
| `IsNonDecreasing`        | Every pair satisfies `a <= b`                  |
| `IsNonIncreasing`        | Every pair satisfies `a >= b`                  |

## Iterators

Lazy `iter.Seq` iterators for zero-allocation pipelines. Use `slices.Collect()` to materialize results into concrete slice values on demand.

| Iterator    | Description                                         |
|-------------|-----------------------------------------------------|
| `Filter`    | Yield elements matching a predicate                 |
| `Map`       | Yield transformed elements                          |
| `Chunk`     | Yield successive sub-slices of a given size         |
| `Values`    | Yield all elements (adapter to `iter.Seq`)          |
| `Backward`  | Yield `(index, element)` pairs in reverse order     |
| `List`      | Yield typed elements from a `container/list.List`   |
| `FilterSeq` | Filter an existing `iter.Seq`                       |
| `MapSeq`    | Transform an existing `iter.Seq`                    |
| `Take`      | Yield at most N elements from an `iter.Seq`         |

## Pool

`Pool[T]` is a generic, concurrency-safe `sync.Pool` wrapper for reusing slice allocations. Slices exceeding 1024 capacity are discarded on return.

`EnsureCapacity` grows a slice to a required capacity, rounding up to the next power of two to reduce future reallocations in append loops.

## Performance

- Slices with 32 or fewer elements use linear scan for deduplication (faster than map-based).
- `MapParallel` / `FilterParallel` use goroutines for large datasets (50K+ elements).
- `FilterFirst`, `Any`, `All`, `Reduce` are zero-allocation with early return.
