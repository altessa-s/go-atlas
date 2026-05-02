# Strings

Concepts for working with `core/text/strings`: string manipulation, case transformation, interning, secure storage, and zero-copy operations.

```
import "github.com/altessa-s/go-atlas/core/text/strings"
```

All utility functions are pure and thread-safe. `SecureString` is safe for concurrent reads. `Interner` is fully thread-safe with lock-free design.

---

## Validation and emptiness

```go
strings.IsEmpty("")         // true
strings.IsEmpty("  ")       // true  (whitespace-only)
strings.IsEmpty[*string](nil) // true

strings.IsEmptyOrWhitespace("  \t") // true — allocation-free, rune-based
```

`IsEmpty` is generic over `~string | ~*string`. `IsEmptyOrWhitespace` avoids the `strings.TrimSpace` allocation by iterating runes directly; prefer it in
hot paths.

### Trimming checks

| Function | Description |
|----------|-------------|
| `IsTrimmed(s)` | No leading/trailing whitespace (ASCII fast path, Unicode fallback) |
| `IsTrimmedUnsafe(s)` | Same, but uses `ToBytesUnsafe` for zero-copy byte access |

---

## Pointer conversion

```go
ptr := strings.ToPtr("hello")  // &"hello"
ptr = strings.ToPtr("")         // nil (empty/whitespace → nil)
s := strings.FromPtr(ptr)       // "hello" (nil → "")
```

`ToPtr` is generic over `~string | ~*string`.

---

## Numeric conversion

Generic `To[T]` parses a string into any numeric type. Returns zero on parse error.

```go
n := strings.To[int64]("42")       // 42
f := strings.To[float64]("3.14")   // 3.14
```

Pre-instantiated converters avoid type parameters at call sites:

| Variable | Type |
|----------|------|
| `ToInt`, `ToInt64`, `ToInt32`, `ToInt16`, `ToInt8` | signed integers |
| `ToUint`, `ToUint64`, `ToUint32`, `ToUint16`, `ToUint8` | unsigned integers |
| `ToFloat32`, `ToFloat64` | floats |

---

## Case transformation

| Function | Example |
|----------|---------|
| `ToSnakeCase(s)` | `"HelloWorld"` → `"hello_world"` |
| `ToScreamingSnakeCase(s)` | `"HelloWorld"` → `"HELLO_WORLD"` |
| `ToCamelCase(s)` | `"hello_world"` → `"helloWorld"` |
| `ScreamingSnakeToCamelCase(s)` | `"HELLO_WORLD"` → `"helloWorld"` |

All transformers use an ASCII fast path with Unicode fallback for non-ASCII input.

### Case checks

| Function | Description |
|----------|-------------|
| `IsLowercase(s)` | All letters are lowercase (rune-based, allocation-free) |
| `IsUppercase(s)` | All letters are uppercase (rune-based, allocation-free) |
| `IsLowercaseUnsafe(s)` | ASCII fast path, Unicode fallback |
| `IsUppercaseUnsafe(s)` | ASCII fast path, Unicode fallback |

---

## Split and Join

### Split

`Split` divides a string with configurable options:

```go
parts := strings.Split("a,,b", strings.SplitOptions{
    Separator: ",",
    SkipEmpty: true,    // removes empty elements
    TrimSpace: true,    // trims whitespace from each part
    MaxSplits: 3,       // limits number of splits (-1 = no limit)
    CaseSensitive: false, // case-insensitive separator matching
})
// ["a", "b"]
```

Case-insensitive splitting uses an optimized path when byte lengths are preserved, falling back to a cached regex for Unicode special casing (bounded at 256
entries).

### SplitSeq (lazy iterator)

`SplitSeq` returns an `iter.Seq[string]` with the same semantics as `Split` but avoids allocating an intermediate slice. Suitable for large inputs or early
termination:

```go
for part := range strings.SplitSeq(largeCSV, strings.SplitOptions{Separator: ","}) {
    if part == target {
        break // no further work
    }
}
```

### Join

```go
result := strings.Join([]string{"a", "", "b"}, strings.JoinOptions{
    Separator: ",",
    SkipEmpty: true,   // "a,b" instead of "a,,b"
    Prefix:    "[",
    Suffix:    "]",
})
// "[a,b]"
```

---

## Contains (advanced search)

```go
r := strings.Contains("Hello World Hello", "hello", strings.ContainsOptions{
    CaseSensitive:   false, // case-insensitive
    MatchWholeWords: true,  // word boundaries only
    Count:           true,  // count all occurrences
})
// r.Found = true, r.Count = 2, r.Positions = [0, 12]
```

When `Count` is false, the search returns on the first match (fast path).

---

## Concatenation

| Function | Description |
|----------|-------------|
| `Concat(parts...)` | Pooled `strings.Builder`, single allocation, pre-grown |
| `ConcatUnsafe(parts...)` | Single `[]byte` allocation + `FromBytesUnsafe` |

Both return the original string for single-element input and empty string for zero elements.

---

## Trim helpers

| Function | Description |
|----------|-------------|
| `TrimPrefixFast(s, prefix)` | Substring-based, never allocates |
| `TrimSuffixFast(s, suffix)` | Substring-based, never allocates |

These use direct substring comparison instead of `strings.TrimPrefix`/`TrimSuffix`.

---

## Zero-copy conversion

```go
b := strings.ToBytesUnsafe("hello")   // []byte — must NOT be modified
s := strings.FromBytesUnsafe(buf)      // string — source must NOT be modified after
```

Both use `unsafe.StringData`/`unsafe.SliceData` (Go 1.20+). The safety contract:
- `ToBytesUnsafe`: returned slice is read-only; modifying it is undefined behavior.
- `FromBytesUnsafe`: source slice must not be modified after conversion.

### Unsafe equality

`StringEqualsUnsafe` compares data pointers first (O(1) for interned strings), then falls back to standard `==` or `strings.EqualFold` if lengths differ.

---

## String interning

The `Interner` deduplicates identical strings so only one copy is retained in memory. Uses a two-tier cache:

1. Hot cache: 32 `atomic.Pointer` slots for the most frequently accessed strings
   (wait-free reads).
2. Cold cache: `sync.Map` for general interning with `unique.Handle` storage.

Strings exceeding 5 accesses are promoted to the hot cache. Background LRU eviction runs when the interner exceeds capacity.

### Global interner

```go
s := strings.InternString("/api/users")  // deduplicated via GlobalInterner()
```

| Convenience function | Description |
|---------------------|-------------|
| `InternString(s)` | Intern as-is |
| `InternLowerString(s)` | Lowercase + intern |
| `InternUpperString(s)` | Uppercase + intern |
| `InternTrimString(s)` | Trim whitespace + intern |
| `InternCleanPathString(s)` | Normalize path (collapse `//`, strip trailing `/`) + intern |
| `InternPrefixString(s, prefix)` | Prepend + intern |
| `InternSuffixString(s, suffix)` | Append + intern |
| `InternWrapString(s, prefix, suffix)` | Wrap + intern |
| `InternStringSlice(slice)` | Intern every element |
| `InternStringMap(m)` | Intern every key and value |
| `InternFormatString(fmt, args...)` | `Sprintf` + intern |
| `InternJoinString(parts, sep)` | Join + intern |
| `InternJoinWith(parts, joiner)` | Custom join + intern |

### Custom interner

```go
interner := strings.NewInterner(4096) // custom capacity

s := interner.String("key")
s = interner.LowerString("KEY")
s = interner.CleanPathString("//foo//bar/")  // "/foo/bar"
```

### Performance monitoring

```go
stats := strings.GlobalInterner().Stats()
fmt.Printf("hit rate: %.1f%% (hot: %.1f%%)\n",
    stats.HitRate()*100, stats.HotHitRate()*100)
fmt.Printf("size: %d/%d, evictions: %d\n",
    stats.CurrentSize, stats.MaxSize, stats.Evictions)
```

| `InternerStats` field | Description |
|----------------------|-------------|
| `HotHits` | Lookups served from hot cache |
| `ColdHits` | Lookups served from cold cache |
| `Misses` | New entries created |
| `Evictions` | Entries removed by LRU |
| `CurrentSize` / `MaxSize` | Current and max capacity |

`ResetStats()` zeroes counters without clearing cached strings.

### Tuning constants

| Constant | Default | Description |
|----------|---------|-------------|
| `DefaultMaxSize` | 8192 | Global interner capacity |
| `HotCacheSlots` | 32 | Atomic pointer slots in hot cache |
| `HotCacheThreshold` | 5 | Access count to trigger hot promotion |
| `PromotionCheckInterval` | 50 | Access count between promotion checks |
| `EvictionBatchSize` | 512 | Max entries per eviction pass |
| `EvictionRatio` | 4 | At least 25% of cache evicted per pass |

---

## Secure strings

`SecureString` stores sensitive data (passwords, tokens, keys) with explicit memory zeroing.

```go
ss := strings.NewSecureString("password")
defer ss.Clear() // zeros memory, returns to pool

value := ss.String()         // allocating copy
value = ss.StringUnsafe()    // zero-copy (invalid after Clear)
data := ss.Bytes()           // allocating copy
defer strings.ZeroBytes(data)
```

### Properties

- Memory zeroing: `Clear()` overwrites stored data with zeros.
- Small-string optimization: strings ≤64 bytes stored inline (no heap allocation).
- Object pooling: instances from `NewSecureString` are recycled via `sync.Pool`.
- GC safety: a runtime cleanup zeros heap data if the instance is collected
  without `Clear`.
- Redacted debug output: `GoString()` returns `SecureString{<redacted>}`.

### Constant-time comparison

```go
if stored.Equal(provided) {
    // timing-safe comparison via crypto/subtle.ConstantTimeCompare
}
```

### Standalone helpers

| Function | Description |
|----------|-------------|
| `SecureCompare(a, b)` | Constant-time string equality |
| `TimingSafePrefixMatch(s, prefix)` | Constant-time case-insensitive prefix check |
| `TimingSafeSubstringMatch(s, substr)` | Constant-time case-insensitive substring search |
| `SubstringMatch(s, substr)` | Case-insensitive substring (NOT constant-time) |
| `ZeroBytes(data)` | Overwrite byte slice with zeros |
| `ZeroString(s)` | Best-effort zeroing of string backing memory (may panic for literals) |

---

## Object pools

### `strings.Builder` pool

```go
b := strings.GetStringBuilder()
defer strings.PutStringBuilder(b)
b.Grow(128)
b.WriteString("hello")
result := b.String()
```

Or use the convenience wrapper:

```go
result := strings.BuildString(func(b *strings.Builder) {
    b.WriteString("hello")
    b.WriteString(" world")
})
```

### String slice pool

```go
s := strings.GetStringSlice()                    // capacity ≥ 64
s = strings.GetStringSliceWithCapacity(256)       // capacity ≥ 256
defer strings.PutStringSlice(s)
```

Element references are cleared on return to allow GC of referenced strings.

---

## See also

- [../runtime/README.md](../runtime/README.md): `AddCleanup` used by `SecureString` for GC safety
