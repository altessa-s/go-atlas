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

| Option                     | Default | Description                       |
|----------------------------|---------|-----------------------------------|
| `WithParserCacheSize`      | 100     | LRU cache capacity for parsed AST |
| `WithParserNoCache`        | --      | Disable expression caching        |

## Translator options

| Option               | Default     | Description                          |
|----------------------|-------------|--------------------------------------|
| `WithAllowedFields`  | all         | Whitelist of queryable field names   |
| `WithFieldMapping`   | identity    | CEL field name to DB column mapping  |
| `WithMaxDepth`       | 20          | Maximum AST nesting depth            |
| `WithStrictMode`     | false       | Fail on unsupported operations       |

## Subpackages

| Package                                        | Description                       |
|------------------------------------------------|-----------------------------------|
| [translators/mongo](./translators/mongo)       | AST to MongoDB `bson.M`           |
| [translators/redisearch](./translators/redisearch) | AST to RediSearch query syntax |
| [translators/lua](./translators/lua)           | AST to Lua boolean expression     |
