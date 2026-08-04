# clickhouse

```go
import "github.com/altessa-s/go-atlas/data/filter/translators/clickhouse"
```

Translates a `filter.Node` AST into a ClickHouse SQL `WHERE` clause. Two output modes share one AST walk: a parameterized clause with `?`
placeholders plus a bound argument slice, and a self-contained clause with literals rendered in place.

## Constructor

| Function                                                     | Description                                                                                       |
|--------------------------------------------------------------|---------------------------------------------------------------------------------------------------|
| `NewTranslator(opts ...filter.TranslatorOption) (*Translator, error)` | Returns `filter.ErrAllowlistRequired` when `WithUntrustedInput` is set without `WithAllowedFields` |

## Methods

| Method                                            | Description                                                                    |
|---------------------------------------------------|--------------------------------------------------------------------------------|
| `Translate(node) (string, []any, error)`          | Parameterized clause; every literal becomes a `?` and is returned in `args`     |
| `TranslateInline(node) (string, error)`           | Self-contained clause with literals rendered in place                           |

A `nil` node translates to `1 = 1` (match all) in both modes, so `"... WHERE " + where` stays valid without a special case.

## Usage

```go
parser, _ := filter.NewParser()
ast, _ := parser.Parse(ctx, `status == 2 && type in [1, 2]`)

trans, err := clickhouse.NewTranslator(
    filter.WithUntrustedInput(),
    filter.WithAllowedFields("status", "type"),
)
if err != nil {
    return err
}

where, args, err := trans.Translate(ast)
// where: (`status` = ?) AND (`type` IN (?, ?))
// args:  []any{int64(2), int64(1), int64(2)}

rows, err := conn.Query(ctx, "SELECT * FROM events WHERE "+where, args...)
```

`TranslateInline` produces `` (`status` = 2) AND (`type` IN (1, 2)) `` for the same AST — use it for materialized view definitions, generated
DDL, logs and debugging, and prefer `Translate` for anything driven by request data.

## Operation mapping

| CEL                          | ClickHouse                       |
|------------------------------|----------------------------------|
| `==` `!=` `<` `>` `<=` `>=`  | `=` `!=` `<` `>` `<=` `>=`       |
| `&&` `\|\|` `!`              | `AND` `OR` `NOT`                 |
| `field`                      | `` `field` = true ``             |
| `field in [a, b]`            | `` `field` IN (?, ?) ``          |
| `field in []`                | `1 = 0`                          |
| `field == null`              | `` `field` IS NULL ``            |
| `field != null`              | `` `field` IS NOT NULL ``        |
| `has(field)`                 | `` `field` IS NOT NULL ``        |
| `field.contains(s)`          | `` position(`field`, ?) > 0 ``   |
| `field.startsWith(s)`        | `` startsWith(`field`, ?) ``     |
| `field.endsWith(s)`          | `` endsWith(`field`, ?) ``       |
| `field.matches(re)`          | `` match(`field`, ?) ``          |
| `field.size()` / `size(f)`   | `` length(`field`) ``            |
| `substring()`                | `filter.ErrUnsupportedOperation` |

`contains` compiles to `position()` rather than `LIKE '%…%'` so that a `%` or `_` in the operand stays literal — there is no pattern escaping
to get wrong. `size()` is rejected outside a comparison: `length()` is an integer expression, and on its own there is no predicate for `WHERE`
to test.

## Column names

Field names are checked against `WithAllowedFields`, run through `WithFieldMapping`, validated as plain ClickHouse identifiers, and emitted
backtick-quoted. Dots survive as part of a single quoted name — `address.city` becomes `` `address.city` ``, which is how Nested columns are
stored.

The validation is stricter than backtick quoting requires: the column position is the one part of the generated SQL that no placeholder can
cover, so anything that is not `[A-Za-z_][A-Za-z0-9_]*` (dot-separated) is rejected with `filter.ErrInvalidExpression`. To filter on a `Map`
key or a computed value, map the CEL name onto a materialized column.

## Null semantics

ClickHouse rows always carry every column of the table, so there is no "missing field" for `has()` to detect — it compiles to `IS NOT NULL`
and is meaningful only against a `Nullable` column. The same three-valued logic applies to `!=`: a NULL row does not satisfy
`` `col` != 'x' ``, because the comparison yields NULL rather than true. Write `col != "x" || col == null` where the NULL row should match.

## Timestamps

`timestamp(...)` literals bind as `time.Time` in the parameterized form and render as `toDateTime64('…', 3, 'UTC')` inline. Inline rendering
is millisecond-precision and drops anything finer; the parameterized form leaves precision to the driver.

## Concurrency

A `Translator` accumulates per-call state (nesting depth, collected arguments) and is **not** safe for concurrent use. Construct one per
goroutine, or guard it with a mutex.

## See also

- [`data/filter`](../..) — parser, AST, and the shared translator options
- [`translators/mariadb`](../mariadb), [`translators/postgres`](../postgres) — the other SQL dialects, sharing the walk in
  [`internal/sqlbase`](../internal/sqlbase)
- [`translators/mongo`](../mongo), [`translators/meili`](../meili), [`translators/redisearch`](../redisearch), [`translators/lua`](../lua)
