# filter

```go
import "github.com/altessa-s/go-atlas/data/filter"
```

Package `filter` parses Common Expression Language (CEL) expressions into an intermediate AST and translates them to database-specific query formats via
the visitor pattern. Includes security features: field allowlists, depth limits, and expression length limits.

## Supported operations

| Category   | Operations                                                 |
|------------|------------------------------------------------------------|
| Comparison | `==`, `!=`, `<`, `<=`, `>`, `>=`                           |
| Logical    | `&&`, `\|\|`, `!`                                          |
| Membership | `in`, `has()`                                              |
| String     | `contains()`, `startsWith()`, `endsWith()`, `matches()`, `substring()`¹ |
| Other      | `size()` (`f.size()` or `size(f)`), `timestamp()`          |

¹ `substring()` is supported by the in-memory evaluator only; translators reject it with `ErrUnsupportedOperation`.

## Key types

| Type / Interface | Description                                             |
|------------------|---------------------------------------------------------|
| `Parser`         | CEL expression parser with LRU caching (`Parse`, `ParseShared`) |
| `Evaluator`      | In-memory evaluation of an AST against a `map[string]any`; safe for concurrent use |
| `Node`           | AST node interface                                      |
| `LiteralNode`    | Literal values                                          |
| `IdentNode`      | Field references                                        |
| `BinaryOpNode`   | Binary operations (comparisons, logical)                |
| `UnaryOpNode`    | Unary operations (negation)                             |
| `CallNode`       | Function/method calls                                   |
| `ListNode`       | List literals                                           |
| `Visitor`        | Interface for traversing and translating AST            |
| `VisitElements`  | Accepts every element of a `ListNode` — the whole of what `VisitList` has to do |
| `DepthGuard`     | Bounds a visitor's recursion; `Enter` / `Leave` / `Reset` |

`VisitElements` and `DepthGuard` exist because every translator in this repository needed them verbatim. A filter arrives from outside the process, so
its nesting is an input like any other: unbounded, a few kilobytes of parentheses become a stack overflow. Take the limit from
`TranslatorContext.MaxDepth`, hold the guard as a field, and pair each `Enter` with a deferred `Leave`:

```go
type Translator struct {
    config *filter.TranslatorContext
    depth  filter.DepthGuard
}

func (t *Translator) VisitBinaryOp(n *filter.BinaryOpNode) (any, error) {
    if err := t.depth.Enter(); err != nil {
        return nil, err
    }
    defer t.depth.Leave()
    // ... dispatch on n.Op
}
```

Custom visitors can call `TranslatorContext.CheckLimits(node)` before walking the AST to enforce the same operation and depth budgets. `AcceptLiteral`
and `VisitLiteralElements` reject field references in positions where a backend accepts only constants.

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

`Parse` returns a caller-owned AST, including copied byte literals. Mutating it cannot change cached results or another caller's tree. Custom-function
results are copied before caching. The evaluator's regex cache holds 256 patterns and evicts least-recently-used entries so a new working set can
replace old patterns.

## Translator options

| Option               | Default  | Description                                                                                                       |
|----------------------|----------|-------------------------------------------------------------------------------------------------------------------|
| `WithAllowedFields`  | all      | Whitelist of queryable field names; keys are the CEL-side names, identical to those used in `WithFieldMapping`    |
| `WithFieldMapping`   | identity | CEL field name to DB column mapping                                                                               |
| `WithFieldTypes`     | --       | Declared kind per field; literals are checked against the declared kind and rejected with `ErrFieldTypeMismatch`  |
| `WithEnumValues`     | --       | Allowed integer value set per enum field; literals outside the set are rejected with `ErrEnumValueNotAllowed`     |
| `WithZeroWhenAbsent` | --       | Kind per field whose absence from a document encodes its zero value; honored by the MongoDB translator            |
| `WithMaxDepth`       | 20       | Maximum AST nesting depth                                                                                         |
| `WithMaxRegexLength` | 1024     | Maximum length of a regex pattern in `matches()`; protects against ReDoS-style payloads                           |
| `WithMaxOperations`  | 1000     | Maximum AST node visits per translation; protects against wide expressions (e.g. hundreds of OR-ed conditions)    |
| `WithUntrustedInput` | --       | Mark translator/evaluator as receiving untrusted input — requires `WithAllowedFields`, otherwise the constructor returns `ErrAllowlistRequired` |

These options also apply to `NewEvaluator`. `WithMaxRegexLength` and `WithMaxOperations` bound evaluation and translation. Translation checks the entire
AST before rendering; evaluation counts visited nodes and retains short-circuit behavior. Lists consume nesting depth too.

Every translator constructor (`mongo.NewTranslator`, `clickhouse.NewTranslator`, `mariadb.NewTranslator`, `postgres.NewTranslator`,
`meili.NewTranslator`, `redisearch.NewTranslator`, `lua.NewTranslator`) and `NewEvaluator` return `(*T, error)`. The error is `ErrAllowlistRequired`
when `WithUntrustedInput` is set without a non-empty `WithAllowedFields`; misconfiguration therefore surfaces at process start, never on the first
request.

### Exact decimals

`FieldKindDecimal` declares a field holding money or another value that must compare exactly. A numeric literal of the expression is read from its
source spelling — the parser keeps the text of a float literal (`LiteralNode.Text`) whenever its float64 would not reproduce it, so
`1.00000000000000001` stays 1.00000000000000001 and never matches 1. Decimal strings (`price >= "12345678901234567.89"`) are accepted too. A float64
that comes from elsewhere (a custom function, a float data value) is read as its shortest round-trip decimal and only with at most 15 significant digits
(`MaxExactFloatDigits`). Every operand, a `decimal.Decimal` data value included, must fit a Decimal128 (34 significant digits, exponent -6176..6111) and
is normalized before any comparison (zero as 0, trailing zeros folded), so an extreme exponent such as `"0e-999999999"` cannot cost unbounded work;
anything else is `ErrFieldTypeMismatch` (`DecimalLiteral`, `DecimalOperand`).

| Consumer             | Behavior                                                                                                         |
|----------------------|------------------------------------------------------------------------------------------------------------------|
| `NewEvaluator`       | A `decimal.Decimal` data value compares exactly with the literal (`0.1+0.2 == 0.3` holds), declared or not      |
| `mongo` translator   | Literals are emitted as BSON `Decimal128`, so MongoDB compares them with stored Decimal128 values exactly         |
| Other translators    | The constructor fails with `ErrUnsupportedType` (`TranslatorContext.RejectFieldKinds`), never compares via float |

```go
eval, err := filter.NewEvaluator(filter.WithAllowedFields("price"),
    filter.WithFieldTypes(map[string]filter.FieldKind{"price": filter.FieldKindDecimal}))
ok, err := eval.Evaluate(node, map[string]any{"price": tender.Price.Amount()}) // decimal.Decimal
```

### Untrusted input

When translating CEL coming from external clients, pair `WithUntrustedInput()` with `WithAllowedFields(...)`. Without an allowlist a hostile client can
filter on any indexed field (e.g. `passwordHash > ""` to enumerate accounts), so the combination "untrusted + no allowlist" is treated as
misconfiguration — `NewTranslator` returns `ErrAllowlistRequired` instead of constructing a deny-all translator.

```go
trans, err := mongo.NewTranslator(
    filter.WithUntrustedInput(),
    filter.WithAllowedFields("name", "status", "createdAt"),
)
if err != nil {
    return err
}
```

## Custom functions

Register CEL functions that the parser expands into arbitrary AST nodes before the filter reaches an evaluator or translator. This is the way to expose
semantic shortcuts such as `createdAfter("2024-01-01")` that map to `createdAt > "2024-01-01"` against the model.

`Constant(field, op, value)` and `CompareField(field, op)` are the two fundamental building blocks. `Constant` exposes a parameter-less predicate
(`name() → field op value`) — the natural shape for status enums and fixed thresholds. `CompareField` exposes a one-argument predicate
(`name(arg) → field op arg`) — the natural shape for "after / before / above / below"-style comparisons over a user-supplied operand:

```go
parser, _ := filter.NewParser(filter.WithCustomFunctions(map[string]filter.CustomFunction{
    "createdAfter":  filter.CompareField("createdAt", filter.OpGT),
    "updatedAfter":  filter.CompareField("updatedAt", filter.OpGT),
    "createdBefore": filter.CompareField("createdAt", filter.OpLT),
    "isActive":      filter.Constant("status", filter.OpEqual, "active"),
    "hasFailures":   filter.Constant("failures", filter.OpGT, int64(0)),
}))
```

Handlers receive already-converted argument nodes and return trees of the built-in node types, including nested `BinaryOpNode` trees. Literals support
scalar values, `time.Time`, and `[]byte`; custom node implementations and other mutable literal types fail explicitly. Functions like
`between(field, lo, hi)` are also expressible.

`TextSearch(fields...)` is a free-text search: `search("text")` expands into `f1.matches(re) || f2.matches(re) || …` with `re = "(?i)" +
regexp.QuoteMeta(trimmed text)`, so it is case-insensitive and literal (no wildcards or anchors), combines with the rest of the filter like any
predicate and goes through the allowlist, the field mapping and the regex checks of the translator. The argument must be one non-blank string literal.
On MongoDB every field is an unanchored `$regex`, which scans (no text index); a NUL byte is spelled `\x00` by the translator.

```go
parser, _ := filter.NewParser(filter.WithCustomFunctions(map[string]filter.CustomFunction{
    "search": filter.TextSearch("code", "name", "description"),
}))
// search("network") && lawCode == "44fz"
```

Names must not collide with built-in CEL functions (`contains`, `startsWith`, `endsWith`, `matches`, `size`, `has`, `timestamp`, `substring`).
`WithFieldMapping` and `WithAllowedFields` operate on the **target** field name (e.g. `createdAt`), since the virtual call is gone after parsing.

### Built-in presets

`TimestampFilters()` returns a ready-made set for the common `createdAt`/`updatedAt`/`deletedAt` predicates — opt in explicitly:

```go
filter.RegisterFunctions(filter.TimestampFilters())
// createAfter(t)  → createdAt > t
// createBefore(t) → createdAt < t
// updateAfter(t)  → updatedAt > t
// updateBefore(t) → updatedAt < t
// deleteAfter(t)  → deletedAt > t
// deleteBefore(t) → deletedAt < t
```

Field names are fixed (camelCase). For snake_case columns add `WithFieldMapping{"createdAt":"created_at",...}` on the translator.

For partial registration use `SelectTimestampFilters(names ...string)`:

```go
filter.RegisterFunctions(filter.SelectTimestampFilters(
    filter.TimestampFuncCreateAfter,
    filter.TimestampFuncUpdateAfter,
))
```

Unknown names are silently skipped — pass the `TimestampFunc*` constants to avoid typos.

`BetweenFilter()` exposes a generic range predicate `between(field, lo, hi)` → `field >= lo && field <= hi`. The first argument must be an identifier;
the bounds can be any literal or expression the translator accepts.

```go
filter.RegisterFunctions(filter.BetweenFilter())
// CEL: between(age, 18, 65)
// CEL: between(price, 100.0, 500.0)
```

`SoftDeleteFilters()` exposes the canonical soft-delete predicates keyed off `deletedAt`:

```go
filter.RegisterFunctions(filter.SoftDeleteFilters())
// notDeleted()  → deletedAt == null
// onlyDeleted() → deletedAt != null
```

Equality with `null` matches both missing and explicitly null values under MongoDB and the in-memory evaluator. If your storage needs a stricter
"absent" check, use `!has(deletedAt)` directly instead.

### Application-wide registration

Register once during bootstrap so every subsequently constructed parser sees the same set without repeating the configuration:

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

`WithCustomFunctions` still works and overrides global entries by name. `WithoutGlobalCustomFunctions()` opts a single parser out of the global set
entirely (useful for tests). Re-registering an existing name returns an error — `ResetGlobalCustomFunctions()` is provided for test isolation.

## Subpackages

| Package                                            | Output                       | Constructor                                                                              |
|----------------------------------------------------|------------------------------|------------------------------------------------------------------------------------------|
| [translators/mongo](./translators/mongo)           | `bson.M`                     | `NewTranslator(opts ...filter.TranslatorOption) (*Translator, error)`                    |
| [translators/clickhouse](./translators/clickhouse) | ClickHouse SQL `WHERE` clause | `NewTranslator(opts ...filter.TranslatorOption) (*Translator, error)`                   |
| [translators/mariadb](./translators/mariadb)       | MariaDB / MySQL `WHERE` clause | `NewTranslator(opts ...filter.TranslatorOption) (*Translator, error)`                  |
| [translators/postgres](./translators/postgres)     | PostgreSQL `WHERE` clause    | `NewTranslator(opts ...filter.TranslatorOption) (*Translator, error)`                    |
| [translators/meili](./translators/meili)           | Meilisearch filter string    | `NewTranslator(opts ...filter.TranslatorOption) (*Translator, error)`                    |
| [translators/redisearch](./translators/redisearch) | RediSearch query string      | `NewTranslator(schema map[string]FieldType, opts ...filter.TranslatorOption) (*Translator, error)` |
| [translators/lua](./translators/lua)               | Lua boolean expression       | `NewTranslator(tableVar string, opts ...filter.TranslatorOption) (*Translator, error)`   |

### SQL translators

`clickhouse`, `mariadb` and `postgres` share one AST walk and differ only in a `Dialect` — see
[`translators/internal/sqlbase`](./translators/internal/sqlbase). Their `Translate` returns three values, `(where string, args []any, err error)`:
literals leave the SQL text entirely and come back as bind arguments aligned with their placeholders, so a clause built from request data cannot alter
the shape of the query. PostgreSQL numbers its placeholders (`$1`, `$2`, …); the other two use positional `?`.

```go
where, args, err := trans.Translate(ast)
rows, err := conn.Query(ctx, "SELECT * FROM events WHERE "+where, args...)
```

`TranslateInline` renders the same clause with literals in place, for the cases a placeholder cannot serve — view definitions, generated DDL, logs. A
`nil` AST translates to `1 = 1` in both modes, so the concatenation above needs no special case.

Dialect differences worth knowing before choosing a filter surface: MariaDB's default collations make every string predicate case-insensitive,
PostgreSQL requires a real `boolean` column for a bare-identifier condition, and only ClickHouse reads a dotted CEL name as a single (Nested) column
rather than a qualified `"table"."column"`. Each package README covers its own.
