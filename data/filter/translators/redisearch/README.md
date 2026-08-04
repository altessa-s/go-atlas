# redisearch

```go
import "github.com/altessa-s/go-atlas/data/filter/translators/redisearch"
```

Package `redisearch` translates filter AST nodes into RediSearch query syntax for server-side filtering on Redis with the RediSearch module.

## Schema

`NewTranslator` takes a `map[string]FieldType` keyed by the column name **after** field mapping is applied. The schema is not advisory: a field's
type decides the shape of every query built against it, and a mismatch between this map and the actual `FT.CREATE` produces queries that are
silently wrong rather than rejected.

| Operation           | `FieldTypeNumeric`               | `FieldTypeTag`         | `FieldTypeText`     |
|---------------------|----------------------------------|------------------------|---------------------|
| `field == v`        | `@field:[v v]`                   | `@field:{v}`           | `@field:(v)`        |
| `field in [a, b]`   | `(@field:[a a]\|@field:[b b])`   | `@field:{a\|b}`        | `@field:(a\|b)`     |
| `field > v`         | `@field:[(v +inf]`               | rejected               | rejected            |

## Bare identifiers

A bare identifier used as a condition becomes a boolean TAG test — `active` translates to `@active:{true}`, `!active` to `-@active:{true}` — at
the root of an expression and on either side of a logical operator. The TAG form is fixed rather than resolved through the schema: a boolean is
only ever indexed as a TAG, and a NUMERIC or TEXT rendering of `true` would not mean anything.

## Not supported

`endsWith()`, `matches()`, `size()` and `has()` have no counterpart in the query syntax and are rejected with
`filter.ErrUnsupportedOperation`. Two further limits belong to RediSearch itself rather than to the translator: an infix `contains()` query needs
the field declared `WITHSUFFIXTRIE`, and a prefix shorter than `MINPREFIX` (2 by default) is dropped, so `startsWith("A")` matches nothing.
