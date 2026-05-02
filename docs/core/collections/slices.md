# Slices

Generic utilities for slice transformation, filtering, deduplication, grouping, ordering, conditional appends, parallel processing, and pooling. Complements the standard library `slices` package with patterns that recur throughout the project.

```go
import coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
```

All pure functions return new slices without modifying inputs. Nil inputs are treated as empty and never panic. Closures passed as predicates or transforms are never copied, so feel free to capture state. `Pool` is safe for concurrent use; `FilterParallel` and `MapParallel` invoke the predicate from multiple goroutines.

> **Import alias:** always import as `coreslices` to avoid the name clash with stdlib `slices`.

---

## API overview

| Symbol group        | Purpose                                                                                                                |
|---------------------|------------------------------------------------------------------------------------------------------------------------|
| Deduplication       | `Deduplicate`, `DeduplicateBy`                                                                                         |
| Removal             | `Delete`                                                                                                               |
| Transformation      | `To`, `ToWithFilter`, `ToAny`, `ToStrings`, `MapParallel`                                                              |
| Filtering           | `FilterFirst`, `FilterLast`, `FilterParallel`                                                                          |
| Aggregation         | `Reduce`, `GroupBy`, `Any`, `All`                                                                                      |
| Ordering checks     | `IsStrictlyIncreasing`, `IsStrictlyDecreasing`, `IsNonDecreasing`, `IsNonIncreasing`                                   |
| Conditional append  | `AppendIf`, `AppendIfFunc`, `AppendNonEmpty`, `AppendNonNil`, `AppendNonNilErr`                                        |
| List interop        | `ToList`, `FromList`                                                                                                   |
| Iterators           | `Filter`, `Map`, `Chunk`, `Values`, `Backward`, `List`, `FilterSeq`, `MapSeq`, `Take`                                  |
| Pooling             | `Pool`, `NewPool`, `EnsureCapacity`                                                                                    |

---

## Deduplication

`Deduplicate` and `DeduplicateBy` return a new slice with duplicates removed, preserving first-occurrence order.

```go
nums := []int{1, 2, 2, 3, 1, 4}
unique := coreslices.Deduplicate(nums) // [1, 2, 3, 4]

people := []Person{{1, "Alice"}, {2, "Bob"}, {1, "Alice2"}, {3, "Charlie"}}
unique := coreslices.DeduplicateBy(people, func(p Person) int { return p.ID })
// [{1, "Alice"}, {2, "Bob"}, {3, "Charlie"}]
```

### Optimizations

- **Copy-on-write** — the input slice is returned as-is when no duplicates are found. The result slice is allocated only on the first detected duplicate.
- **Consecutive-duplicate fast path** — adjacent duplicates are detected with a `lastKey` cache, skipping the map lookup.
- **Bounded pre-allocation** — the seen-set map is sized at `min(len(collection), 128)` so very large slices with frequent duplicates do not over-allocate.
- **Identity wrapper** — `Deduplicate` is a thin wrapper around `DeduplicateBy(s, identity)`; both share the same code path.

---

## Removal

`Delete` removes the first occurrence of an element, returning the original slice unchanged when the element is absent. It delegates to `slices.Delete` after locating the index, so the cost is `O(n)` plus a single shift.

```go
words := []string{"a", "b", "c"}
result := coreslices.Delete(words, "b") // ["a", "c"]
```

For removing all occurrences, use `slices.DeleteFunc` from the standard library.

---

## Transformation

| Function       | Description                                                              |
|----------------|--------------------------------------------------------------------------|
| `To`           | Apply `fn` to each element; returns a new slice                          |
| `ToWithFilter` | Filter and transform in a single pass — no intermediate slice            |
| `ToAny`        | Box a typed slice into `[]any`                                           |
| `ToStrings`    | Extract string-typed elements from `[]any`, skipping non-strings         |
| `MapParallel`  | Like `To`, but parallelized for large inputs                             |

```go
nums := []int{1, 2, 3}
strs := coreslices.To[[]int, int, string](nums, strconv.Itoa) // ["1", "2", "3"]

evenStrs := coreslices.ToWithFilter(nums,
    func(n int) bool { return n%2 == 0 },
    func(n int) string { return fmt.Sprintf("even:%d", n) },
) // ["even:2"]
```

`ToWithFilter` is the canonical way to chain a filter + transform without paying for the intermediate slice. The result is `nil` when no element passes, matching the `FilterMap` convention from the maps package.

`To` uses `~[]E` constraints so named slice types (`type IDs []int`) pass through without explicit conversion.

---

## Filtering

| Function         | Description                                                       |
|------------------|-------------------------------------------------------------------|
| `FilterFirst`    | First element matching the predicate; zero-allocation, early-exit |
| `FilterLast`     | Last element matching the predicate; zero-allocation, early-exit  |
| `FilterParallel` | Concurrent filter for large inputs (order may not be preserved)   |
| `Filter` (iter)  | Lazy `iter.Seq` filter — see [Iterators](#iterators)              |

```go
nums := []int{1, 2, 3, 4, 5}
firstEven, ok := coreslices.FilterFirst(nums, func(n int) bool { return n%2 == 0 }) // 2, true
lastEven, _ := coreslices.FilterLast(nums, func(n int) bool { return n%2 == 0 })    // 4, true
```

### `FilterParallel` characteristics

- Below `32 * NumCPU * 2` elements (e.g., ~512 on an 8-core machine) it falls back to the sequential lazy `Filter` — no goroutine overhead.
- Above that threshold it dispatches to `core/runtime/concurrency.ProcessCollect`, returning items in input order with respect to dispatch but with a sentinel `errFiltered` for rejected entries (filtered out before return).
- The predicate must be safe for concurrent invocation.

---

## Aggregation

```go
sum := coreslices.Reduce(nums, 0, func(acc, _ int, val int) int { return acc + val })

people := []Person{{"Alice", 30}, {"Bob", 25}, {"Charlie", 30}}
groups := coreslices.GroupBy(people, func(p Person) int { return p.Age })
// {25: [Bob], 30: [Alice, Charlie]}

hasEven := coreslices.Any(nums, func(n int) bool { return n%2 == 0 })
allEven := coreslices.All(nums, func(n int) bool { return n%2 == 0 })
```

- **`Reduce`** is zero-allocation — the accumulator stays on the stack when `R` is small.
- **`GroupBy`** pre-sizes the internal map with a load-factor estimate (`len * 0.75`, floored at 16) to reduce rehashing as groups fill.
- **`Any` / `All`** short-circuit. `All` returns `true` for an empty slice (vacuous truth) — match Go's convention.
- `Any` delegates to `slices.ContainsFunc` from the standard library.

---

## Ordering checks

```go
coreslices.IsStrictlyIncreasing([]int{1, 2, 3}) // true
coreslices.IsStrictlyDecreasing([]int{3, 2, 1}) // true
coreslices.IsNonDecreasing([]int{1, 2, 2, 3})   // true (equal values allowed)
coreslices.IsNonIncreasing([]int{3, 2, 2, 1})   // true
```

All four take any `cmp.Ordered` element type and short-circuit on the first violation. Empty and single-element slices return `true` for every check (vacuously satisfied).

---

## Conditional append

The `Append*` family is the project-wide replacement for `if cond { s = append(s, x) }`. They keep factory and builder chains expression-shaped (one `opts = ...` line per concept) and match the existing style across the repo.

| Function           | When to use                                                               |
|--------------------|---------------------------------------------------------------------------|
| `AppendIf`         | Cheap value(s) guarded by a `bool`                                        |
| `AppendIfFunc`     | Non-trivial construction that should not run when the condition is false  |
| `AppendNonEmpty`   | String-keyed pair lists where the value must be non-empty                 |
| `AppendNonNil`     | Guard against typed-nil interfaces and nil pointers                       |
| `AppendNonNilErr`  | Lazy construction with error propagation                                  |

```go
opts := []Option{WithFoo(cfg.Foo)}
opts = coreslices.AppendIf(opts, cfg.Verbose, WithVerbose())
opts = coreslices.AppendIfFunc(opts, cfg.HasTLS(), func() []Option {
    return []Option{WithTLS(cfg.TLS.Cert, cfg.TLS.Key)}
})

attrs := []any{"trace_id", traceID}
attrs = coreslices.AppendNonEmpty(attrs, "user", req.User)

handlers := []Handler{authHandler}
handlers = coreslices.AppendNonNil(handlers, optionalMetricsHandler())
```

`AppendNonNil` uses `core/types/nilcheck.IsNil`, which correctly distinguishes typed-nil interfaces (`var err error = (*pathError)(nil)`) from a true nil. Bare `value != nil` does not catch that case, and it's a well-known footgun.

`AppendIf` evaluates its variadic values eagerly. When evaluation is expensive or unsafe (dereferencing a maybe-nil pointer), use `AppendIfFunc`, which calls `fn` only after checking `cond`.

The plain `if cond { s = append(s, x) }` form is acceptable only when the branch contains additional logic that doesn't fit a single `Append*` call.

---

## List interop

`container/list.List` adapters for the rare cases where `list.List` shows up at an API boundary:

```go
nums := []int{1, 2, 3}
l := coreslices.ToList(nums)        // *list.List

back := coreslices.FromList[int](l) // [1, 2, 3]
```

`FromList` performs a per-element type assertion to `T` and silently skips elements that don't match. Use the `List` iterator (below) when you want to walk a `*list.List` lazily without building a slice at all.

---

## Iterators

Lazy `iter.Seq` / `iter.Seq2` producers (Go 1.23+). Construction allocates nothing. Iteration is lazy, so early termination is free and pipelines stay zero-allocation.

| Iterator    | Description                                       |
|-------------|---------------------------------------------------|
| `Filter`    | Yield elements matching a predicate               |
| `Map`       | Yield transformed elements                        |
| `Chunk`     | Yield successive sub-slices of size `n`           |
| `Values`    | Yield all elements (adapter to `iter.Seq`)        |
| `Backward`  | Yield `(index, element)` pairs in reverse order   |
| `List`      | Yield typed elements from `*list.List`            |
| `FilterSeq` | Filter an existing `iter.Seq`                     |
| `MapSeq`    | Transform an existing `iter.Seq`                  |
| `Take`      | Yield at most N elements from an `iter.Seq`       |

```go
nums := []int{1, 2, 3, 4, 5}

for n := range coreslices.Filter(nums, isEven) {
    // ...
}

for chunk := range coreslices.Chunk(nums, 2) {
    process(chunk) // [1 2], [3 4], [5]
}

// Compose pipeline with no intermediate slices.
seq := coreslices.MapSeq(
    coreslices.FilterSeq(coreslices.Values(nums), isEven),
    func(n int) string { return fmt.Sprintf("n=%d", n) },
)
result := slices.Collect(coreslices.Take(seq, 100))
```

`Chunk` shares the underlying array with `collection`. Sub-slices are views, not copies, so mutating a yielded chunk mutates the source. This is intentional for high-throughput batch processing; copy explicitly when you need isolation.

`Backward` is an `iter.Seq2[int, T]` so the yielded index matches the original position, not the reverse-iteration step.

---

## Pool

`Pool[T]` is a generic wrapper around `sync.Pool` for reusing `[]T` slice buffers in hot paths.

```go
var bufPool = coreslices.NewPool[byte](256)

func handle(req *Request) {
    bufPtr := bufPool.Get()
    defer bufPool.Put(bufPtr)

    *bufPtr = append(*bufPtr, req.Body...)
    process(*bufPtr)
}
```

### Behavior

- **`Get`** resets the slice length to zero while preserving the underlying capacity. Cold path (pool empty): allocates `make([]T, 0, defaultCap)`.
- **`GetWithCapacity(n)`** replaces the pooled slice with a freshly allocated one when `cap(*slice) < n` and `n <= 1024`. Use it when you know an approximate upper bound and want to avoid grow-and-copy churn during `append`.
- **`Put`** calls `clear` on the slice (so referenced values can be GC'd), resets the length, and returns it to the pool. Slices with `cap > 1024` are discarded so the pool does not retain runaway buffers.
- **Sentinel value** — `0` or negative `defaultCap` is replaced with `64`.

### `EnsureCapacity`

Standalone helper for growing a slice to a required capacity, rounding up to the next power of two for amortized append efficiency:

```go
buf := make([]int, 0, 4)
coreslices.EnsureCapacity(&buf, 100) // grows to capacity 128
```

Returns `true` when a new backing array was allocated. The power-of-two rounding only applies up to `maxSlicePoolCapacity` (1024); larger requests get exactly the requested capacity.

### Best practices

- **Always pair with `defer Put`** — leaking a pooled slice back to the GC defeats the purpose.
- **Don't share** — never hand a pooled slice to another goroutine; ownership is bounded by `Get` / `Put`.
- **Match `defaultCap` to the median request size.** Too small and `GetWithCapacity` reallocates often; too large and every pooled instance wastes memory.
- **Don't pool slices of pointer-heavy structs without thinking about GC** — `clear` handles the slice elements, but the values they reference may still be live until the next pool turnover.

---

## Best practices

### Reach for the iterator variants when chaining

```go
// Good — single allocation at the end.
strs := slices.Collect(coreslices.MapSeq(
    coreslices.FilterSeq(coreslices.Values(nums), isHot),
    formatItem,
))

// Worse — one intermediate slice per stage.
hot := slices.Collect(coreslices.Filter(nums, isHot))
strs := coreslices.To[[]int, int, string](hot, formatItem)
```

The materialized variants (`To`, `ToWithFilter`, etc.) exist for cases where the result is the artifact (cache value, return type, JSON payload).

### Use `ToWithFilter` to avoid two passes

`ToWithFilter` is roughly twice as fast as `Filter` followed by `To` for non-trivial slice sizes, because it skips the intermediate slice and walks the input exactly once.

### Pick the right `FilterFirst` / `FilterLast` / `Filter`

- **`FilterFirst` / `FilterLast`** — you only need the match. Zero allocation, early return.
- **`slices.IndexFunc`** (stdlib) — you only need the index.
- **`Filter`** — you need every match and want to chain.

### `Any` / `All` over manual loops

```go
// Idiomatic.
ok := coreslices.All(items, isReady)

// Verbose.
ok := true
for _, it := range items {
    if !it.IsReady() { ok = false; break }
}
```

### Use `MapParallel` / `FilterParallel` only on large inputs

The threshold (`32 * NumCPU * 2` elements, e.g. ~512 on 8 cores) is set so the goroutine setup cost is amortized over enough work to pay back. Below that, sequential wins. Don't second-guess the threshold without a benchmark.

The predicate / transformation must be safe for concurrent invocation. `MapParallel` preserves input order; `FilterParallel` does not necessarily.

### Use the `Append*` helpers in factory chains

The project standard for conditional appends is the `Append*` family. The `if`-style is acceptable only when the branch contains additional logic that doesn't fit a single helper call. Reference style: `transport/grpc/server/factory/builder.go`, `data/audit/factory/builder.go`, `auth/oidc/factory/builder.go`.

```go
// Good — expression-shaped.
opts := []Option{WithFoo(foo)}
opts = coreslices.AppendIf(opts, cfg.Verbose, WithVerbose())
opts = coreslices.AppendIfFunc(opts, cfg.HasTLS(), func() []Option {
    return tlsOptions(cfg.TLS) // only constructed when needed
})

// Avoid — same logic in imperative form.
opts := []Option{WithFoo(foo)}
if cfg.Verbose {
    opts = append(opts, WithVerbose())
}
if cfg.HasTLS() {
    opts = append(opts, tlsOptions(cfg.TLS)...)
}
```

### Treat `nil` and empty as equivalent

Every function in this package treats `nil` slice input as empty. Returns are also `nil` when the result is empty, so `len(result) == 0` is the only check callers need.

### Always alias the import

```go
import coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
```

The bare name collides with stdlib `slices`; the project-wide alias is `coreslices`.

---

## Performance notes

- `Deduplicate` / `DeduplicateBy` are copy-on-write — no allocation when the input is already unique. Map pre-allocation is bounded at 128 to avoid over-allocation when duplicates are common.
- A consecutive-duplicate fast path skips the map lookup when adjacent elements share a key, which is common in sorted inputs.
- For tiny slices (≤32 elements) a linear scan with a slice-of-seen-keys is roughly 25× faster than a map; the package's lookup helpers internally pick the right path.
- `FilterFirst`, `FilterLast`, `Any`, `All`, `Reduce` are zero-allocation and short-circuit on first match / first failure.
- `MapParallel` chunks the work statically over `NumCPU` goroutines via `sync.WaitGroup.Go` (Go 1.25+); no semaphore overhead.
- `FilterParallel` delegates to `core/runtime/concurrency.ProcessCollect`, which respects context cancellation and uses a channel-based semaphore.
- `GroupBy` pre-sizes the internal map at `len * 0.75` (rounded up to a minimum of 16) so the typical fill never triggers a rehash.
- `ToAny` / `To` / `MapParallel` allocate the output slice exactly once, sized to the input length.
- `Pool.Put` clears the slice with the built-in `clear` (Go 1.21+) so pooled buffers do not pin old element values past their useful lifetime.
- `EnsureCapacity` rounds up to the next power of two below the 1024 cap, matching the runtime's growth heuristic for amortized `O(1)` appends.

---

## See also

- [maps.md](maps.md) — companion package with the same conventions for map operations.
- `core/runtime/concurrency` — the engine behind `FilterParallel`; use it directly for fan-out beyond simple `Map` / `Filter`.
- `core/collections/slices/README.md` — package-level reference next to the source.
