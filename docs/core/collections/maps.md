# Maps

Generic utilities for map transformation, filtering, conversion, and high-performance read-only storage. Complements the standard library `maps`
package with merging, swapping, flat-key expansion, weak references, and a Swiss-table-based `ImmutableMap`.

```go
import coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
```

All pure functions return new maps without modifying inputs. Nil inputs are treated as empty and never panic. `ImmutableMap`, `WeakMap`, and `Pool`
are safe for concurrent use; `WeakRef` is a value type with the same lifetime semantics as `weak.Pointer`.

> **Import alias:** always import as `coremaps` to avoid the name clash with stdlib `maps`.

---

## API overview

| Symbol group        | Purpose                                                              |
|---------------------|----------------------------------------------------------------------|
| Pure transformers   | `Merge`, `Swap`, `FilterMap`, `ConvertMap`                           |
| Slice interop       | `FromSlice`, `FromSliceWith`, `ToKeyValueSlice`                      |
| Flat-key conversion | `FromFlatMap`, `FromFlatMapWithHandler`, `ToFlatMap`                 |
| Iterators           | `Keys`, `Values`, `Filter`, `Map`                                    |
| Read-only storage   | `ImmutableMap`, `NewImmutableMap`, `NewImmutableMapFromEntries`      |
| Weak references     | `WeakMap`, `NewWeakMap`, `WeakRef`, `MakeWeakRef`                    |
| Pooling             | `Pool`, `NewPool`                                                    |

---

## Pure functions

### Merge

`Merge(src, dst)` returns a new map combining both inputs. Keys in `src` overwrite keys in `dst`. Neither input is mutated; the result is freshly
allocated and pre-sized to `len(src) + len(dst)`.

```go
defaults := map[string]string{"theme": "light", "lang": "en"}
userPrefs := map[string]string{"theme": "dark", "timezone": "UTC"}
merged := coremaps.Merge(userPrefs, defaults)
// {"lang": "en", "theme": "dark", "timezone": "UTC"}
```

When one input is empty, `Merge` returns a copy of the other (never the original) so callers never receive an aliased map.

### Swap

`Swap` transposes keys and values. Both `K` and `V` must be `comparable`. If `src` contains duplicate values, exactly one of the corresponding keys
is retained. Which one wins is non-deterministic, since Go randomizes map iteration order.

```go
codes := map[string]int{"error": 500, "success": 200}
swapped := coremaps.Swap(codes) // map[int]string{200: "success", 500: "error"}
```

### FilterMap and ConvertMap

`FilterMap` keeps entries for which the predicate returns true; `ConvertMap` rewrites both key and value through a transformation function.

```go
ages := map[string]int{"alice": 25, "bob": 17, "charlie": 30}
adults := coremaps.FilterMap(ages, func(_ string, age int) bool { return age >= 18 })

userIDs := map[string]int{"alice": 1, "bob": 2}
idToName := coremaps.ConvertMap(userIDs, func(name string, id int) (int, string) { return id, name })
```

Use the lazy iterator variants (`Filter`, `Map`) when the result feeds into another pipeline stage and you want to skip the intermediate allocation.

### Slice interop

| Function          | Description                                                                  |
|-------------------|------------------------------------------------------------------------------|
| `FromSlice`       | Build a map by extracting a key from each element; element becomes the value |
| `FromSliceWith`   | Build a map by extracting both key and value from each element               |
| `ToKeyValueSlice` | Flatten a map to `[]any{k1, v1, k2, v2, …}` for variadic logger APIs         |

```go
users := []User{{ID: 1, Name: "Alice"}, {ID: 2, Name: "Bob"}}
byID := coremaps.FromSlice(users, func(u User) int { return u.ID })

tags := map[string]string{"env": "prod", "region": "us-east"}
logger.With(coremaps.ToKeyValueSlice(tags)...)
```

`ToKeyValueSlice` pre-sizes the result to `len(m)*2` so the `slog.Logger.With(...)` happy path is single-allocation.

---

## Flat-key conversion

`FromFlatMap` expands dot-separated keys into a nested `map[string]any` hierarchy. `ToFlatMap` is the inverse.

```go
flat := map[string]any{
    "user.name":         "Alice",
    "user.age":          25,
    "user.address.city": "NYC",
}
nested := coremaps.FromFlatMap(flat)
// {"user": {"name": "Alice", "age": 25, "address": {"city": "NYC"}}}

back := coremaps.ToFlatMap(nested, nil)
// {"user.name": "Alice", "user.age": 25, "user.address.city": "NYC"}
```

### Conflict handling

When a key path traverses through a non-map value, the conflicting entry is silently skipped. Pass a `ConflictHandler` to observe these cases:

```go
flat := map[string]any{
    "user":      "scalar",        // user is a leaf
    "user.name": "Alice",         // attempts to traverse through "scalar"
}

conflicts := 0
nested := coremaps.FromFlatMapWithHandler(flat, func(key string, existing any) {
    logger.Warn("flat-map conflict", slog.String("key", key), slog.Any("existing", existing))
    conflicts++
})
```

The handler is observability-only — entries are always skipped on conflict. The hot loop uses `strings.IndexByte` (single-byte SIMD scan) instead of
`strings.Split`, so no intermediate slices are allocated per key.

### `keyMapper`

`ToFlatMap` accepts an optional segment transformer. Pass `nil` for an identity transform; otherwise use it for casing or interning:

```go
flat := coremaps.ToFlatMap(nested, strings.ToLower)
```

---

## Iterators

`Keys`, `Values`, `Filter`, and `Map` return `iter.Seq` / `iter.Seq2` producers (Go 1.23+). They allocate nothing during construction and yield entries
lazily, which means iterations can be terminated early without paying for the unyielded items.

```go
for k := range coremaps.Keys(m) {
    // ...
}

// Materialize when you need a slice.
keys   := slices.Collect(coremaps.Keys(m))
sorted := slices.Sorted(coremaps.Keys(m))

// Compose without intermediate maps.
for k, v := range coremaps.Filter(m, func(_ string, v int) bool { return v > 0 }) {
    // ...
}
```

Use `Filter` / `Map` over `FilterMap` / `ConvertMap` whenever the result is consumed exactly once. The materialized variants exist for callers that
need a true `map[K]V` (cache, return value, JSON marshal target).

---

## ImmutableMap

`ImmutableMap[K, V]` is a read-only hash map built once from a standard Go map or two parallel slices. After construction it cannot be modified and is
safe for concurrent reads without synchronization or defensive copies.

```go
src := map[string]int{"a": 1, "b": 2, "c": 3}
m := coremaps.NewImmutableMap(src)

v, ok := m.Get("a")    // 1, true
present := m.Contains("b")
size := m.Len()
```

### Why use it

| Property                  | Standard `map`              | `ImmutableMap`                                                     |
|---------------------------|-----------------------------|--------------------------------------------------------------------|
| Concurrent reads          | Safe only with `sync.RWMutex` or under build-once-publish discipline | Always safe, no lock                       |
| Memory overhead at scale  | Pointer-heavy bucket layout | Three flat slices; ~1.5–2× lower memory at >10K entries            |
| GC scan cost              | Walks every bucket pointer  | Three slice headers; constant pointer count regardless of size     |
| Allocation count          | Grows with rehashing        | Exactly 3 heap allocations regardless of size                      |
| Lookup speed              | Comparable                  | Comparable on Go 1.24+ (also Swiss-table internally)               |
| Mutation                  | Yes                         | None — `Set`/`Delete` do not exist                                 |

### When to use

- Package-level lookup tables: `var codes = coremaps.NewImmutableMap(map[string]struct{}{…})`
- Struct fields populated in a constructor and never modified (registries, codec mappings, field schemas)
- Configuration maps that are built from options and frozen before use
- Any map you would otherwise comment with `// read-only` or `// do not modify`

### When NOT to use

- Maps that grow lazily at runtime (caches, `sync.Map`-backed stores)
- Maps modified after construction (use `sync.RWMutex` + plain `map` or `sync.Map`)
- Ephemeral maps local to a function — plain `map` has zero ceremony cost

### Construction

`NewImmutableMap` copies from a source `map[K]V`; the source is not retained and may be reused or discarded by the caller. `NewImmutableMapFromEntries`
accepts parallel slices and panics on length mismatch. Useful when entries arrive from a streaming source:

```go
keys := []string{"a", "b", "c"}
vals := []int{1, 2, 3}
m := coremaps.NewImmutableMapFromEntries(keys, vals)
```

### Iteration

`All`, `Keys`, `Values` return `iter.Seq2` / `iter.Seq` lazy iterators, matching the rest of the package:

```go
for k, v := range m.All() {
    // ...
}

sortedKeys := slices.Sorted(m.Keys())
```

Iteration order is hash-dependent and not guaranteed.

### How it works

`ImmutableMap` uses an open-addressing Swiss-table layout:

- **Group size 8** — control bytes are packed into one `uint64` per group; lookup uses SWAR (SIMD Within A Register) bit-tricks to scan all 8 lanes
  in a single integer comparison, no branches.
- **H2 fingerprint** — the low 7 bits of the hash are stored as a per-slot fingerprint; bit 7 distinguishes empty slots. This lets `Get` reject most
  candidate slots without touching the keys array.
- **87.5% target load factor** — the table is sized to `n*8/7` slots, rounded up to a multiple of 8. Higher density than the runtime map, with
  predictable probe sequences.
- **`hash/maphash` per-instance seed** — randomizes the layout so adjacent maps don't share collision patterns.

The data is laid out as three contiguous slices (`ctrl`, `keys`, `vals`), which the GC scans as plain arrays. There are no overflow chains or
per-bucket pointers, so the scan time is O(1) regardless of population.

### Freeze pattern

When a type has a mutable registration phase followed by a long read-only phase, store both a `map` (writes) and an `*ImmutableMap` (reads). Add a
`Freeze()` method that builds the `ImmutableMap` and nils the mutable map. `Register` after `Freeze` should panic. The reference implementation lives
in `transport/grpc/interceptors/auth.ScopeRegistry`.

---

## WeakMap

`WeakMap[K, V]` is a concurrency-safe map that holds `weak.Pointer` references to its values. Entries are automatically removed when the GC reclaims
the referenced value via `runtime.AddCleanup` — no periodic sweeps, no background goroutines, no manual eviction policy.

```go
cache := coremaps.NewWeakMap[string, *Session]()

cache.Set("session-123", session)

if s, ok := cache.Get("session-123"); ok {
    use(s)
}

cache.Range(func(k string, s *Session) bool {
    // visits only entries whose values are still alive
    return true
})
```

### Properties

- **GC-driven eviction** — when a value loses all strong references, the cleanup callback removes the corresponding entry on the next GC cycle. No
  CPU overhead between cycles.
- **Approximate `Len`** — may include entries whose values have already been collected but whose cleanup callbacks haven't fired yet. Call
  `Cleanup()` first if an exact count is needed.
- **Snapshot iteration** — `Range` takes a snapshot under a write lock, then iterates without holding any lock; `f` may safely call other `WeakMap`
  methods.
- **Set(nil)** — equivalent to `Delete`. Set to a non-nil value re-arms the cleanup; the old cleanup callback no-ops because it checks pointer
  identity before deleting.

### When to use

- Caches where a separate strong reference (e.g., a request handler holding a `*Session`) determines lifetime, and the cache should not extend it.
- Indices into a primary collection where the primary collection can drop entries at any time.
- Per-object metadata that should disappear with the object.

### When NOT to use

- Any cache whose hit rate must be predictable. GC timing is not.
- Caches where eviction must run on a clock (TTLs). Use `data/cache` or a dedicated TTL store instead.

### `WeakRef`

`WeakRef[T]` is a thin wrapper around `weak.Pointer[T]` for a single weak reference. The zero value is "already dead":

```go
ref := coremaps.MakeWeakRef(obj)
if v := ref.Value(); v != nil {
    // still alive
}
```

`IsAlive` returns the current liveness, but the result may be invalidated immediately after the call returns. Always prefer `Value() != nil` and hold
on to the returned pointer for the duration of the operation.

---

## Pool

`Pool[K, V]` is a generic, concurrency-safe wrapper around `sync.Pool` for reusing `map[K]V` allocations.

```go
var attrPool = coremaps.NewPool[string, any](128)

func handle(req *Request) {
    attrs := attrPool.Get()
    defer attrPool.Put(attrs)

    (*attrs)["user"] = req.User
    (*attrs)["path"] = req.Path
    logger.With(coremaps.ToKeyValueSlice(*attrs)...).Info("request")
}
```

### Behavior

- **`Get`** clears the map before handing it out, so callers always receive an empty map. If the pool is empty, a fresh map of `defaultCap` capacity
  is allocated.
- **`GetWithCapacity(n)`** asks for at least `n` buckets. Requests within `defaultCap` reuse the pooled allocation directly. Larger requests discard
  the pooled map and allocate a freshly-sized one — Go does not expose a map's underlying capacity, so a previously-grown pooled map cannot be
  distinguished from a baseline-sized one.
- **`Put`** clears the map and returns it. Maps with `len(*m) > 1024` are discarded so the pool does not retain runaway allocations.
- **Sentinel value** — `0` or negative `defaultCap` is replaced with `64`.

### Best practices

- **Always pair with `defer Put`** — leaking a pooled map back to the GC defeats the purpose.
- **Don't share** — never hand a pooled map to another goroutine; ownership is bounded by `Get` / `Put`.
- **Don't exceed 1024 entries** in steady state, or the pool drops everything you return and effectively becomes a no-op.

---

## Best practices

### Prefer iterators when chaining

```go
// Good — single allocation at the end.
out := slices.Collect(coremaps.Filter(m, isHot))

// Worse — intermediate map.
out := coremaps.FilterMap(m, isHot)
```

`FilterMap` / `ConvertMap` exist for cases where the result is the artifact (cache value, return type, JSON payload). Otherwise prefer the lazy
variants.

### Reach for `ImmutableMap` for build-once-read-many

If a map is constructed in a factory or `init` and never written afterward, `ImmutableMap` is the explicit lock-free contract. It documents the intent
in the type itself: there's no `// do not modify` comment to rot.

### Use `WeakMap` only when GC timing is acceptable

`WeakMap` is the right tool when "evict eventually after the value is unreferenced" is the policy you want. It is the wrong tool when there is a
deadline ("evict after 60s of idle"). Mixing the two surprises future readers; reach for `data/cache` instead.

### Pool maps in hot paths only

`sync.Pool` overhead is non-trivial for cold paths. Pool only when allocation profiling identifies a hot path that constructs and discards the same
shape of map every request.

### Treat `nil` and empty as equivalent

Every function in this package treats `nil` map input as empty. Returns are also `nil` when the result is empty, so `len(result) == 0` is the only
check callers need.

### Always alias the import

```go
import coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
```

The bare name collides with stdlib `maps`; the project-wide alias is `coremaps`.

---

## Performance notes

- `Merge`, `Swap`, `FilterMap`, `ConvertMap`, `FromSlice`, `FromSliceWith` pre-size the result map from the input length to avoid rehashing.
- `FilterMap` pre-sizes to `len/2` on the assumption that filters reject roughly half. Past that, normal map growth kicks in, still cheaper than
  starting at `0`.
- `ToKeyValueSlice` pre-sizes the result to `len(m)*2` and appends in a single pass.
- `FromFlatMap` walks each key with `strings.IndexByte` and reuses the substring view, so there's no `strings.Split` allocation per key.
- `ImmutableMap.Get` is allocation-free: the SWAR `matchByte` runs in a few cycles and short-circuits on the first empty slot in the probe sequence
  (early termination because the table is build-once: there are no tombstones).
- `ImmutableMap` has exactly 3 heap allocations regardless of size (`ctrl`, `keys`, `vals` slices), versus the runtime map's growing bucket array.
- `WeakMap.Range` snapshots under a write lock so user code in `f` cannot deadlock against further `Set` / `Delete` calls.
- `Pool.Put` clears the map before re-pooling, so `Get` is `O(1)` instead of paying the clear cost on the consumer side.

---

## See also

- [slices.md](slices.md) — companion package with the same conventions for slice operations.
- `core/runtime/concurrency` — bounded fan-out for parallelizing per-entry work over a map.
- `core/collections/maps/README.md` — package-level reference next to the source.
