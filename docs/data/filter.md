# Filter (`filter`)

```go
import "github.com/altessa-s/go-atlas/data/filter"
```

A CEL-based filter engine: parse a Common Expression Language string into an intermediate AST, then either evaluate it in-memory against a
`map[string]any` or translate it to a database query (`bson.M` for MongoDB, RediSearch syntax, or Lua boolean for Redis `EVAL`). One CEL
expression — multiple back ends. Custom CEL functions let you expose semantic shortcuts (`createdAfter("2024-01-01")`) without leaking storage
field names into the API.

---

## When to reach for `filter`

| Scenario | Use |
|---|---|
| Accept a user-defined query string and run it against MongoDB | `Parser` + `translators/mongo.NewTranslator` |
| In-memory predicate over decoded data (config gating, in-process search) | `Parser` + `Evaluator` |
| Server-side filter on plain Redis keys via `EVAL` | `translators/lua` |
| Server-side filter on a RediSearch index | `translators/redisearch` |
| Expose semantic API filters (`createdAfter`, `between`) that map to real columns | `WithCustomFunctions` + `CompareField` |
| Untrusted CEL from end users | `WithUntrustedInput` + `WithAllowedFields` (mandatory pair) |
| Decouple API field names from DB columns | `WithFieldMapping` |

---

## Quick start

### Parser → MongoDB query

```go
parser, err := filter.NewParser()
if err != nil { return err }

ast, err := parser.Parse(ctx, `name == "John" && age >= 18`)
if err != nil { return err }

trans := mongo.NewTranslator()
bsonFilter, err := trans.Translate(ast)
// → {"$and":[{"name":"John"},{"age":{"$gte":18}}]}
```

### Parser → in-memory evaluator

```go
parser, _ := filter.NewParser()
ast, _ := parser.Parse(ctx, `status == "active" && size(tags) > 0`)

eval := filter.NewEvaluator()
ok, err := eval.Evaluate(ast, map[string]any{
    "status": "active",
    "tags":   []any{"vip"},
})
// ok == true
```

### Untrusted input (HTTP query parameter)

```go
trans := mongo.NewTranslator(
    filter.WithUntrustedInput(),
    filter.WithAllowedFields("name", "status", "createdAt"),
    filter.WithMaxDepth(10),
)
```

`WithUntrustedInput` makes the allowlist mandatory — without `WithAllowedFields` the translator refuses to run, instead of letting a hostile
client filter on `passwordHash > ""` to enumerate accounts.

---

## CEL syntax

| Category | Operators / functions |
|---|---|
| Comparison | `==`, `!=`, `<`, `<=`, `>`, `>=` |
| Logical | `&&`, `\|\|`, `!` |
| Membership | `in`, `has(field)` |
| String | `field.contains(s)`, `field.startsWith(s)`, `field.endsWith(s)`, `field.matches(regex)` |
| Other | `field.size()`, `timestamp("RFC3339 string")` |
| Nested fields | dot notation: `address.city == "NYC"` |

`timestamp()` is parsed at AST construction time and stored as `time.Time` in a `LiteralNode` — it is not deferred to the translator.

---

## Custom functions

Register CEL functions that the parser expands into AST nodes before evaluation or translation. This is how you keep the API free of storage
field names: a client sends `createdAfter("2024-01-01")` and the parser rewrites it to `createdAt > "2024-01-01"`. Translators never see the
virtual name.

`CompareField(field, op)` is the canonical building block for the common "function with one argument becomes `field op arg`" shape:

```go
parser, _ := filter.NewParser(filter.WithCustomFunctions(map[string]filter.CustomFunction{
    "createdAfter":  filter.CompareField("createdAt", filter.OpGT),
    "updatedAfter":  filter.CompareField("updatedAt", filter.OpGT),
    "createdBefore": filter.CompareField("createdAt", filter.OpLT),
}))
```

Multi-argument and compound expansions are fine — the handler can return any `Node`:

```go
between := func(args []filter.Node) (filter.Node, error) {
    if len(args) != 3 {
        return nil, fmt.Errorf("between: 3 args, got %d", len(args))
    }
    field := args[0]
    return &filter.BinaryOpNode{
        Op:    filter.OpAnd,
        Left:  &filter.BinaryOpNode{Op: filter.OpGTE, Left: field, Right: args[1]},
        Right: &filter.BinaryOpNode{Op: filter.OpLTE, Left: field, Right: args[2]},
    }, nil
}
```

Constraints:

- Names cannot collide with built-ins (`contains`, `startsWith`, `endsWith`, `matches`, `size`, `has`, `timestamp`). Validation runs at
  `NewParser` — misconfiguration fails fast.
- Handlers run on every parser cache miss; keep them cheap and side-effect free.
- The parser does not bound the depth of trees a handler returns. `WithMaxDepth` enforces the cap during translator/evaluator traversal.
- The expanded AST contains the **target** field (`createdAt`), so `WithFieldMapping` and `WithAllowedFields` operate on that name, not on
  the virtual one.

### Application-wide registration

When the same set of custom functions should be visible to every parser in the application, register them once during bootstrap:

```go
func init() {
    if err := filter.RegisterFunctions(map[string]filter.CustomFunction{
        "createdAfter":  filter.CompareField("createdAt", filter.OpGT),
        "updatedAfter":  filter.CompareField("updatedAt", filter.OpGT),
    }); err != nil {
        panic(err)
    }
}

// Anywhere — the parser picks the global registry up by default.
parser, _ := filter.NewParser()
```

| API | Purpose |
|---|---|
| `RegisterFunctions(m)` | Add to the package-level registry. Re-registering a name returns an error |
| `GlobalCustomFunctions()` | Snapshot of the registry for inspection or debugging |
| `ResetGlobalCustomFunctions()` | Test helper — clears the registry; not for production code |
| `WithoutGlobalCustomFunctions()` | Parser opt-out: ignore the global registry for this parser only |

Per-parser `WithCustomFunctions` still overrides global entries by name — explicit configuration beats the default. Tests that mutate the
global registry must use `ResetGlobalCustomFunctions()` (e.g. via `t.Cleanup`) and must not run in parallel with each other.

---

## Translators

| Package | Output | Use when |
|---|---|---|
| [`translators/mongo`](../../data/filter/translators/mongo) | `bson.M` | MongoDB collection scan or aggregation `$match` |
| [`translators/redisearch`](../../data/filter/translators/redisearch) | RediSearch query string | Server-side filter on a Redis hash with the RediSearch module |
| [`translators/lua`](../../data/filter/translators/lua) | Lua boolean expression | Filter plain Redis keys via `EVAL` |

All translators implement `filter.Visitor` and accept the same `TranslatorOption` set (allowlist, field mapping, depth limit, untrusted-input
guard, strict mode).

---

## Security limits

| Option | Default | What it bounds |
|---|---|---|
| `WithMaxExpressionLength` | 4096 bytes | Reject before parsing — protects parser memory and the LRU cache |
| `WithParserCacheSize` | 1000 entries | LRU cache for parsed AST |
| `WithMaxDepth` | 20 levels | AST nesting during translation/evaluation |
| `WithMaxOperations` | 1000 visits | Total node visits per evaluation — kills wide flat ORs |
| `WithMaxRegexLength` | 1024 bytes | Pattern length passed to `matches()` |
| `WithAllowedFields` | (none) | Whitelist of queryable field names |
| `WithAllowedFunctions` | (none) | Whitelist of callable function names (built-in and custom). Operators and `has()` always allowed |
| `WithUntrustedInput` | off | Marks input as user-supplied; refuses to run without an allowlist |
| `WithStrictMode` | off | Fail (vs. ignore) on unsupported operations |

Length limits and depth caps belong on the translator/evaluator config (`TranslatorOption`); cache size and expression length live on the parser
config (`ParserOption`). Don't try to stretch defaults — if you genuinely need 50-deep filters, the model is wrong.

---

## Field mapping

Map CEL field names to storage column names without rewriting expressions:

```go
trans := mongo.NewTranslator(
    filter.WithFieldMapping(map[string]string{
        "userName":  "user_name",
        "createdAt": "created_at",
    }),
)
```

The mapping applies after custom-function expansion, so the chain
`createdAfter` (custom) → `createdAt` (parser AST) → `created_at` (translator) works automatically.

---

## Errors

| Error | When |
|---|---|
| `ErrParseFailed` | CEL grammar / syntax errors at parse time |
| `ErrEmptyExpression` | Empty or whitespace-only input |
| `ErrExpressionTooLong` | Input above `WithMaxExpressionLength` |
| `ErrInvalidExpression` | Structurally invalid AST (wrong arity, nil node from custom handler, etc.) |
| `ErrUnsupportedOperation` | Operator/function not supported by the target translator (e.g., regex `matches` on Lua) |
| `ErrUnsupportedType` | Literal type not representable in the target backend |
| `ErrFieldNotAllowed` | Field outside `WithAllowedFields` |
| `ErrFunctionNotAllowed` | Function outside `WithAllowedFunctions` (built-in or custom) |
| `ErrAllowlistRequired` | `WithUntrustedInput` set without `WithAllowedFields` |
| `ErrMaxDepthExceeded` | AST nesting above `WithMaxDepth` |
| `ErrMaxOperationsExceeded` | Visits above `WithMaxOperations` |
| `ErrInvalidRegex` | Pattern fails RE2 compilation or exceeds `WithMaxRegexLength` |

All sentinels match via `errors.Is`. Don't string-match error messages.

---

## Metrics

Subsystem `filter`:

| Metric | Type | Description |
|---|---|---|
| `filter_parse_duration_seconds` | histogram | Parse latency (cache hit ≈ 0) |
| `filter_parse_errors_total` | counter | Parse failures (any cause) |
| `filter_translations_total{target_backend}` | counter | Translations performed, labeled by backend (`mongo`, `redisearch`, `lua`) |

Pass a collector via `WithParserCollector(c)`. Translators do not emit translations themselves yet — wire `translations_total{target_backend=...}`
in your call site if you need per-backend visibility. Regex pattern cache stats are exposed in code via
`filter.RegexCacheStatsSnapshot()` for ad-hoc inspection; they are not Prometheus-exported.

---

## Failure modes

**`ErrUnsupportedOperation` from a translator at runtime.** Not every operator/function is implementable in every backend (Lua doesn't do
regex; RediSearch doesn't do `size()` over arbitrary types). Either narrow the queryable set with `WithAllowedFields` and document the API
contract, or enable `WithStrictMode(true)` and surface the failure to the API layer up front.

**Cache-poisoning by deep input.** `WithMaxExpressionLength` is the first line of defense — long expressions never enter the LRU cache. Keep
the limit conservative for untrusted callers (under 1 KiB is plenty for typical filters).

**"My custom function isn't firing."** Check three things in order: (1) `NewParser` returned no error (a name collision or nil handler fails
construction), (2) the function is invoked as a call (`createdAfter("...")`), not a comparison (`createdAfter == "..."`), (3) handlers run on
parser cache misses only — disable the cache with `WithParserNoCache` while debugging or invalidate by changing the expression text.

**`ErrAllowlistRequired` in production but not staging.** The translator was built with `WithUntrustedInput()` somewhere (rightly), but no
`WithAllowedFields` was wired through that environment's config. The error fires at the first untrusted query, not at startup — explicit
allowlists per-environment beat shared defaults.

**Translator + custom function disagreement.** `WithFieldMapping` and `WithAllowedFields` operate on the **target** field name produced by
the custom function, not on the virtual call name. If you whitelist `createdAfter` you'll get `ErrFieldNotAllowed` for `createdAt`.

**`ErrFunctionNotAllowed` for a function you registered.** `WithAllowedFunctions` and `WithCustomFunctions` are independent — registering a
handler does not auto-allow the name. When a whitelist is configured, every callable name (built-ins like `contains` and your custom
functions) must appear in it. This is intentional: it lets one global parser registry serve multiple endpoints, each narrowing the visible
function set on its own.

**Test sees a function it didn't register.** A previous test (or `init()`) registered the function via `RegisterFunctions` and the global
state leaked. Add `t.Cleanup(filter.ResetGlobalCustomFunctions)` (or use `WithoutGlobalCustomFunctions()` on the parser under test) and do
not run global-registry tests in parallel with each other.

---

## Related docs

- [`data/filter/README.md`](../../data/filter/README.md) — godoc-level option reference
- [`docs/configuration.md`](../configuration.md) — overall YAML format
- [`docs/metrics.md`](../metrics.md) — full metrics reference for the repository
- [`data/mongo`](../../data/mongo) — pairs naturally with `translators/mongo` for repository code
