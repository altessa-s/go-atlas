# strings

```go
import "github.com/altessa-s/go-atlas/core/text/strings"
```

Package `strings` provides string manipulation, conversion, case transformation, interning, secure storage, and pooling utilities. All
utility functions are pure, allocation-aware, and safe for concurrent use from multiple goroutines.

## Validation and conversion

| Function                    | Description                                               |
|-----------------------------|-----------------------------------------------------------|
| `IsEmpty`                   | True if string/`*string` is empty or whitespace (generic) |
| `IsEmptyOrWhitespace`       | Same for plain strings (allocation-free)                  |
| `ToPtr`                     | String to `*string`, nil when empty                       |
| `FromPtr`                   | `*string` to string, `""` when nil                        |
| `To[T]`                     | Parse string to any numeric type                          |
| `ToInt64`, `ToFloat64`, ... | Pre-instantiated numeric converters                       |

## Case and trimming

| Function                            | Description                            |
|-------------------------------------|----------------------------------------|
| `ToSnakeCase`                       | `"HelloWorld"` -> `"hello_world"`      |
| `ToCamelCase`                       | `"hello_world"` -> `"helloWorld"`      |
| `ToScreamingSnakeCase`              | `"HelloWorld"` -> `"HELLO_WORLD"`      |
| `IsLowercase` / `IsUppercase`       | Check letter case (Unicode-aware)      |
| `IsTrimmed`                         | True if no leading/trailing whitespace |
| `TrimPrefixFast` / `TrimSuffixFast` | Zero-allocation prefix/suffix removal  |

## Join and Split

Configurable via `JoinOptions`, `SplitOptions`, and `ContainsOptions` structs. All operations support case sensitivity and empty-value skipping.

| Function   | Description                                                  |
|------------|--------------------------------------------------------------|
| `Join`     | Concatenate with separator, prefix/suffix, skip-empty        |
| `Split`    | Split with case sensitivity, max splits, trim, skip-empty    |
| `SplitSeq` | Lazy `iter.Seq` variant of `Split` (zero intermediate alloc) |
| `Contains` | Search with case, whole-word, and count options              |

## Concatenation

| Function       | Description                                  |
|----------------|----------------------------------------------|
| `Concat`       | Pooled `strings.Builder`, single allocation  |
| `ConcatUnsafe` | Single `[]byte` allocation, zero-copy result |

## Security

| Function                   | Description                                      |
|----------------------------|--------------------------------------------------|
| `SecureCompare`            | Constant-time string equality                    |
| `TimingSafePrefixMatch`    | Constant-time, case-insensitive prefix check     |
| `TimingSafeSubstringMatch` | Constant-time, case-insensitive substring search |
| `SubstringMatch`           | Case-insensitive substring (NOT constant-time)   |

## SecureString

Tamper-resistant storage for sensitive data (passwords, tokens, keys). Zeroes memory on `Clear()`, uses inline storage for strings up
to 64 bytes, and returns instances to a pool to reduce allocations in high-throughput code paths.

| Method                  | Description                       |
|-------------------------|-----------------------------------|
| `String`                | Safe copy of stored data          |
| `StringUnsafe`          | Zero-copy (invalid after `Clear`) |
| `Bytes` / `BytesUnsafe` | Byte slice accessors              |
| `Equal`                 | Constant-time comparison          |
| `Clear`                 | Zero memory and return to pool    |

## Interner

Lock-free, LRU-evicting string deduplication. Two-tier cache: hot (atomic slots) + cold (`sync.Map`). Reduces memory for repeated string values.

| Function / Method             | Description                 |
|-------------------------------|-----------------------------|
| `InternString`                | Intern via global singleton |
| `NewInterner`                 | Create a sized interner     |
| `String`                      | Deduplicate a string        |
| `LowerString` / `UpperString` | Case-convert and intern     |
| `TrimString`                  | Trim whitespace and intern  |
| `StringSlice` / `StringMap`   | Bulk intern                 |

## Pooling

| Function              | Description                                      |
|-----------------------|--------------------------------------------------|
| `GetStringSlice`      | Borrow a `[]string` from the pool                |
| `PutStringSlice`      | Return a `[]string` to the pool                  |
| `GetStringBuilder`    | Borrow a `strings.Builder` from the pool         |
| `PutStringBuilder`    | Return a `strings.Builder` to the pool           |
| `BuildString`         | Borrow, build, return in one call                |

## Zero-copy (unsafe)

| Function             | Description                                        |
|----------------------|----------------------------------------------------|
| `ToBytesUnsafe`      | String to `[]byte` without copy (**read-only**)    |
| `FromBytesUnsafe`    | `[]byte` to string without copy (**don't modify**) |
| `StringEqualsUnsafe` | Pointer-compare interned strings, then fallback    |
