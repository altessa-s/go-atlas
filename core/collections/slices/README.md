# slices

```go
import "github.com/altessa-s/go-atlas/core/collections/slices"
```

Package `slices` provides generic utilities for slice transformation, filtering, deduplication, grouping, and pooling. Functions return new slices
without modifying inputs — except `Delete`, which removes in place like stdlib `slices.Delete`, and the `Append*` / `EnsureCapacity` helpers, which grow
the slice they are given; nil slices are treated as empty and never cause panics.

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
| `List`      | Yield typed elements from a `container/list.List`   |
| `FilterSeq` | Filter an existing `iter.Seq`                       |
| `MapSeq`    | Transform an existing `iter.Seq`                    |
| `Take`      | Yield at most N elements from an `iter.Seq`         |

## Pool

`Pool[T]` (`NewPool(defaultCap)`, default capacity 64) is a generic, concurrency-safe `sync.Pool` wrapper for reusing slice allocations. `Get` /
`GetWithCapacity` hand out a zero-length `*[]T`; `Put` returns it, discarding slices exceeding 1024 capacity.

`EnsureCapacity` grows a slice to a required capacity, rounding up to the next power of two (for requests up to 1024; larger ones get the exact
capacity) to reduce future reallocations in append loops.

## Performance

- `Deduplicate` / `DeduplicateBy` use a map-based seen set with a fast path for consecutive duplicates, and return the input slice unchanged (no result
  allocation) when it has no duplicates.
- `MapParallel` / `FilterParallel` split the work across `runtime.NumCPU()` goroutines once the input reaches `64 × NumCPU` elements; smaller inputs run
  sequentially.
- `FilterFirst`, `All`, `Reduce` are zero-allocation; `FilterFirst` and `All` return early, `Reduce` visits every element.
