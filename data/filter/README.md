# filter

```go
import "github.com/altessa-s/go-atlas/data/filter"
```

Package `filter` parses Common Expression Language (CEL) expressions into an intermediate AST and translates them to database-specific query formats
via the visitor pattern. Includes security features: field allowlists, depth limits, and expression length limits.

## Supported operations

| Category   | Operations                                                 |
|------------|------------------------------------------------------------|
| Comparison | `==`, `!=`, `<`, `<=`, `>`, `>=`                           |
| Logical    | `&&`, `\|\|`, `!`                                          |
| Membership | `in`, `has()`                                              |
| String     | `contains()`, `startsWith()`, `endsWith()`, `matches()`, `substring()`¹ |
| Other      | `size()`, `timestamp()`                                    |

¹ `substring()` is supported by the in-memory evaluator only; translators reject it with `ErrUnsupportedOperation`.

## Key types

| Type / Interface | Description                                             |
|------------------|---------------------------------------------------------|
| `Parser`         | CEL expression parser with LRU caching                  |
| `Node`           | AST node interface                                      |
| `BinaryOpNode`   | Binary operations (comparisons, logical)                |
| `UnaryOpNode`    | Unary operations (negation)                             |
| `CallNode`       | Function/method calls                                   |
| `ListNode`       | List literals                                           |
| `Visitor`        | Interface for traversing and translating AST            |

## Parser options

| Option                         | Default | Description                                        |
|--------------------------------|---------|----------------------------------------------------|
| `WithParserCacheSize`          | 1000    | LRU cache capacity for parsed AST                  |
| `WithParserNoCache`            | --      | Disable expression caching                         |
| `WithMaxExpressionLength`      | 4096    | Maximum CEL expression length in bytes; longer expressions are rejected before parsing |
| `WithAllowedFunctions`         | all     | Whitelist of callable function names (excludes operators and `has`) |
| `WithCustomFunctions`          | --      | Register custom CEL functions (see below)          |
| `WithoutGlobalCustomFunctions` | --      | Skip the package-level registry (see below)        |
| `WithParserCollector`          | --      | Prometheus metrics collector for parse latency, error counters, and regex-cache stats |

## Translator options

| Option                 | Default  | Description                                        |
|------------------------|----------|----------------------------------------------------|
| `WithAllowedFields`    | all      | Whitelist of queryable field names                 |
| `WithFieldMapping`     | identity | CEL field name to DB column mapping                |
| `WithMaxDepth`         | 20       | Maximum AST nesting depth                          |
| `WithMaxRegexLength`   | 1024     | Maximum length of a regex pattern in `matches()`; protects against ReDoS-style payloads |
| `WithMaxOperations`    | 1000     | Maximum AST node visits per translation; protects against wide expressions (e.g. hundreds of OR-ed conditions) |
| `WithStrictMode(bool)` | false    | Fail on unsupported operations                     |
| `WithUntrustedInput`   | --       | Mark translator/evaluator as receiving untrusted input — requires `WithAllowedFields`, otherwise `Translate` returns `ErrAllowlistRequired` |

These options also apply to `NewEvaluator`. `WithMaxRegexLength` and
`WithMaxOperations` are the evaluator's primary DoS guards.

### Untrusted input

When translating CEL coming from external clients, pair
`WithUntrustedInput()` with `WithAllowedFields(...)`. Without an
allowlist a hostile client can filter on any indexed field
(e.g. `passwordHash > ""` to enumerate accounts), so the combination
"untrusted + no allowlist" is treated as misconfiguration —
`Translate` returns `ErrAllowlistRequired` instead of proceeding.

```go
trans := mongo.NewTranslator(
    filter.WithUntrustedInput(),
    filter.WithAllowedFields("name", "status", "createdAt"),
)
```

## Custom functions

Register CEL functions that the parser expands into arbitrary AST nodes
before the filter reaches an evaluator or translator. This is the way to
expose semantic shortcuts such as `createdAfter("2024-01-01")` that map
to `createdAt > "2024-01-01"` against the model.

`Constant(field, op, value)` and `CompareField(field, op)` are the two
fundamental building blocks. `Constant` exposes a parameter-less
predicate (`name() → field op value`) — the natural shape for status
enums and fixed thresholds. `CompareField` exposes a one-argument
predicate (`name(arg) → field op arg`) — the natural shape for
"after / before / above / below"-style comparisons over a user-supplied
operand:

```go
parser, _ := filter.NewParser(filter.WithCustomFunctions(map[string]filter.CustomFunction{
    "createdAfter":  filter.CompareField("createdAt", filter.OpGT),
    "updatedAfter":  filter.CompareField("updatedAt", filter.OpGT),
    "createdBefore": filter.CompareField("createdAt", filter.OpLT),
    "isActive":      filter.Constant("status", filter.OpEqual, "active"),
    "hasFailures":   filter.Constant("failures", filter.OpGT, int64(0)),
}))
```

Handlers receive already-converted argument nodes and may return any
node — including nested `BinaryOpNode` trees — so multi-argument
functions like `between(field, lo, hi)` are also expressible.

Names must not collide with built-in CEL functions (`contains`,
`startsWith`, `endsWith`, `matches`, `size`, `has`, `timestamp`,
`substring`).
`WithFieldMapping` and `WithAllowedFields` operate on the **target**
field name (e.g. `createdAt`), since the virtual call is gone after
parsing.

### Built-in presets

`TimestampFilters()` returns a ready-made set for the common
`createdAt`/`updatedAt`/`deletedAt` predicates — opt in explicitly:

```go
filter.RegisterFunctions(filter.TimestampFilters())
// createAfter(t)  → createdAt > t
// createBefore(t) → createdAt < t
// updateAfter(t)  → updatedAt > t
// updateBefore(t) → updatedAt < t
// deleteAfter(t)  → deletedAt > t
// deleteBefore(t) → deletedAt < t
```

Field names are fixed (camelCase). For snake_case columns add
`WithFieldMapping{"createdAt":"created_at",...}` on the translator.

For partial registration use `SelectTimestampFilters(names ...string)`:

```go
filter.RegisterFunctions(filter.SelectTimestampFilters(
    filter.TimestampFuncCreateAfter,
    filter.TimestampFuncUpdateAfter,
))
```

Unknown names are silently skipped — pass the `TimestampFunc*` constants to avoid typos.

`BetweenFilter()` exposes a generic range predicate
`between(field, lo, hi)` → `field >= lo && field <= hi`. The first
argument must be an identifier; the bounds can be any literal or
expression the translator accepts.

```go
filter.RegisterFunctions(filter.BetweenFilter())
// CEL: between(age, 18, 65)
// CEL: between(price, 100.0, 500.0)
```

`SoftDeleteFilters()` exposes the canonical soft-delete predicates
keyed off `deletedAt`:

```go
filter.RegisterFunctions(filter.SoftDeleteFilters())
// notDeleted()  → deletedAt == null
// onlyDeleted() → deletedAt != null
```

Equality with `null` matches both missing and explicitly null values
under MongoDB and the in-memory evaluator. If your storage needs a
stricter "absent" check, use `!has(deletedAt)` directly instead.

### Application-wide registration

Register once during bootstrap so every subsequently constructed
parser sees the same set without repeating the configuration:

```go
func init() {
    err := filter.RegisterFunctions(map[string]filter.CustomFunction{
        "createdAfter":  filter.CompareField("createdAt", filter.OpGT),
        "updatedAfter":  filter.CompareField("updatedAt", filter.OpGT),
    })
    if err != nil { panic(err) }
}

// Anywhere in the app — picks up the global registry by default.
parser, _ := filter.NewParser()
```

`WithCustomFunctions` still works and overrides global entries by
name. `WithoutGlobalCustomFunctions()` opts a single parser out of the
global set entirely (useful for tests). Re-registering an existing
name returns an error — `ResetGlobalCustomFunctions()` is provided for
test isolation.

## Subpackages

| Package                                            | Output                       | Constructor                                                            |
|----------------------------------------------------|------------------------------|------------------------------------------------------------------------|
| [translators/mongo](./translators/mongo)           | `bson.M`                     | `NewTranslator(opts ...filter.TranslatorOption)`                       |
| [translators/meili](./translators/meili)           | Meilisearch filter string    | `NewTranslator(opts ...filter.TranslatorOption)`                       |
| [translators/redisearch](./translators/redisearch) | RediSearch query string      | `NewTranslator(schema map[string]FieldType, opts ...filter.TranslatorOption)` |
| [translators/lua](./translators/lua)               | Lua boolean expression       | `NewTranslator(tableVar string, opts ...filter.TranslatorOption)`      |
