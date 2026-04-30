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
| String     | `contains()`, `startsWith()`, `endsWith()`, `matches()`   |
| Other      | `size()`, `timestamp()`                                    |

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

| Option                     | Default | Description                                        |
|----------------------------|---------|----------------------------------------------------|
| `WithParserCacheSize`      | 100     | LRU cache capacity for parsed AST                  |
| `WithParserNoCache`        | --      | Disable expression caching                         |
| `WithCustomFunctions`      | --      | Register custom CEL functions (see below)          |

## Translator options

| Option               | Default     | Description                          |
|----------------------|-------------|--------------------------------------|
| `WithAllowedFields`  | all         | Whitelist of queryable field names   |
| `WithFieldMapping`   | identity    | CEL field name to DB column mapping  |
| `WithMaxDepth`       | 20          | Maximum AST nesting depth            |
| `WithStrictMode`     | false       | Fail on unsupported operations       |

## Custom functions

Register CEL functions that the parser expands into arbitrary AST nodes
before the filter reaches an evaluator or translator. This is the way to
expose semantic shortcuts such as `createdAfter("2024-01-01")` that map
to `createdAt > "2024-01-01"` against the model.

`CompareField(field, op)` is the canonical building block for the common
"function with one argument becomes `field op arg`" shape:

```go
parser, _ := filter.NewParser(filter.WithCustomFunctions(map[string]filter.CustomFunction{
    "createdAfter":  filter.CompareField("createdAt", filter.OpGT),
    "updatedAfter":  filter.CompareField("updatedAt", filter.OpGT),
    "createdBefore": filter.CompareField("createdAt", filter.OpLT),
}))
```

Handlers receive already-converted argument nodes and may return any
node — including nested `BinaryOpNode` trees — so multi-argument
functions like `between(field, lo, hi)` are also expressible.

Names must not collide with built-in CEL functions (`contains`,
`startsWith`, `endsWith`, `matches`, `size`, `has`, `timestamp`).
`WithFieldMapping` and `WithAllowedFields` operate on the **target**
field name (e.g. `createdAt`), since the virtual call is gone after
parsing.

## Subpackages

| Package                                        | Description                       |
|------------------------------------------------|-----------------------------------|
| [translators/mongo](./translators/mongo)       | AST to MongoDB `bson.M`           |
| [translators/redisearch](./translators/redisearch) | AST to RediSearch query syntax |
| [translators/lua](./translators/lua)           | AST to Lua boolean expression     |
