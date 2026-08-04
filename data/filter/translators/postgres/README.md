# postgres

```go
import "github.com/altessa-s/go-atlas/data/filter/translators/postgres"
```

Translates a `filter.Node` AST into a PostgreSQL `WHERE` clause. Two output modes share one AST walk: a parameterized clause with numbered
`$n` placeholders plus a bound argument slice, and a self-contained clause with literals rendered in place.

## Constructor

| Function                                                              | Description                                                                                        |
|-----------------------------------------------------------------------|----------------------------------------------------------------------------------------------------|
| `NewTranslator(opts ...filter.TranslatorOption) (*Translator, error)` | Returns `filter.ErrAllowlistRequired` when `WithUntrustedInput` is set without `WithAllowedFields` |

## Methods

| Method                                   | Description                                                                  |
|------------------------------------------|------------------------------------------------------------------------------|
| `Translate(node) (string, []any, error)` | Parameterized clause; every literal becomes a `$n` and is returned in `args`  |
| `TranslateInline(node) (string, error)`  | Self-contained clause with literals rendered in place                         |

A `nil` node translates to `1 = 1` (match all) in both modes, so `"... WHERE " + where` stays valid without a special case.

## Usage

```go
parser, _ := filter.NewParser()
ast, _ := parser.Parse(ctx, `status == 2 && type in [1, 2]`)

trans, err := postgres.NewTranslator(
    filter.WithUntrustedInput(),
    filter.WithAllowedFields("status", "type"),
)
if err != nil {
    return err
}

where, args, err := trans.Translate(ast)
// where: ("status" = $1) AND ("type" IN ($2, $3))
// args:  []any{int64(2), int64(1), int64(2)}

rows, err := conn.Query(ctx, "SELECT * FROM events WHERE "+where, args...)
```

**Placeholders are numbered from `$1`,** counting up in the order the arguments are collected. A clause cannot be spliced into a query that
already binds parameters without renumbering — build the whole `WHERE` from one `Translate` call, or append the returned args after your own
and shift accordingly.

## Operation mapping

| CEL                          | PostgreSQL                             |
|------------------------------|----------------------------------------|
| `==` `!=` `<` `>` `<=` `>=`  | `=` `!=` `<` `>` `<=` `>=`             |
| `&&` `\|\|` `!`              | `AND` `OR` `NOT`                       |
| `field`                      | `"field" = TRUE`                       |
| `field in [a, b]`            | `"field" IN ($1, $2)`                  |
| `field in []`                | `1 = 0`                                |
| `field == null`              | `"field" IS NULL`                      |
| `field != null`              | `"field" IS NOT NULL`                  |
| `has(field)`                 | `"field" IS NOT NULL`                  |
| `field.contains(s)`          | `strpos("field", $1) > 0`              |
| `field.startsWith(s)`        | `starts_with("field", $1)`             |
| `field.endsWith(s)`          | `right("field", length($1)) = $2`      |
| `field.matches(re)`          | `"field" ~ $1`                         |
| `field.size()` / `size(f)`   | `length("field")`                      |
| `substring()`                | `filter.ErrUnsupportedOperation`       |

`contains` and `startsWith` take the needle as a plain string — there is no LIKE pattern, so a `%` or `_` in the operand stays literal.
`starts_with()` requires PostgreSQL 11 or newer. `size()` is rejected outside a comparison: `length()` is an integer expression, and on its own
there is no predicate for `WHERE` to test.

**`endsWith` binds its operand twice.** PostgreSQL has no `endsWith`, so the needle both sizes the suffix and is compared to it. One CEL
argument consumes two placeholders — `args` reflects that, and the numbering of everything after it shifts accordingly.

## Column names

Field names are checked against `WithAllowedFields`, run through `WithFieldMapping`, validated as plain SQL identifiers, and emitted
double-quoted per dot-separated segment: `address.city` becomes `"address"."city"`.

The validation is stricter than quoting requires: the column position is the one part of the generated SQL that no placeholder can cover, so
anything that is not `[A-Za-z_][A-Za-z0-9_]*` (dot-separated) is rejected with `filter.ErrInvalidExpression`. To filter on a jsonb path or a
computed value, map the CEL name onto a generated column.

Quoting also pins the case. PostgreSQL folds unquoted identifiers to lower case, so an unquoted `createdAt` would resolve to the column
`createdat`; quoted, it means exactly `createdAt`. Where the table uses snake_case, say so with `WithFieldMapping` rather than relying on
folding.

## Types

PostgreSQL is stricter than the other backends in this family:

- A **bare identifier** used as a condition compiles to `"col" = TRUE` and requires an actual `boolean` column — there is no
  integer-to-boolean coercion.
- **`size()`** compiles to `length()`, which covers `text`. Array and `jsonb` columns need `cardinality()` and `jsonb_array_length()` instead,
  so map those fields onto a generated column if you need to filter on their size.

## Null semantics

A SQL row always carries every column of its table, so there is no "missing field" for `has()` to detect — it compiles to `IS NOT NULL` and is
meaningful only against a nullable column. The same three-valued logic applies to `!=`: a NULL row does not satisfy `"col" != 'x'`, because the
comparison yields NULL rather than true. Write `col != "x" || col == null` where the NULL row should match.

## Regular expressions

`matches()` compiles to the case-sensitive POSIX operator `~`. PostgreSQL's engine backtracks, so a crafted pattern can be made expensive
server-side. `filter.WithMaxRegexLength` bounds the blast radius but does not eliminate it — expose `matches()` to trusted callers, or tighten
the cap. A `statement_timeout` is the reliable backstop.

## Timestamps

`timestamp(...)` literals bind as `time.Time` in the parameterized form and render as a UTC `TIMESTAMP WITH TIME ZONE` literal at microsecond
precision inline.

## Inline rendering

`TranslateInline` emits escape-string literals, `E'...'`. The prefix is what makes one escaping rule correct on any server: inside a plain
`'...'` literal the backslash is an escape character only when `standard_conforming_strings` is off, so no plain-literal escaping is correct in
both settings.

## Concurrency

A `Translator` accumulates per-call state (nesting depth, collected arguments) and is **not** safe for concurrent use. Construct one per
goroutine, or guard it with a mutex.

## See also

- [`data/filter`](../..) — parser, AST, and the shared translator options
- [`translators/mariadb`](../mariadb), [`translators/clickhouse`](../clickhouse) — the other SQL dialects
