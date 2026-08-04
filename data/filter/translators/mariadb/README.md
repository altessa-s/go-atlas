# mariadb

```go
import "github.com/altessa-s/go-atlas/data/filter/translators/mariadb"
```

Translates a `filter.Node` AST into a MariaDB SQL `WHERE` clause. Two output modes share one AST walk: a parameterized clause with `?`
placeholders plus a bound argument slice, and a self-contained clause with literals rendered in place. The output is equally valid MySQL.

## Constructor

| Function                                                             | Description                                                                                        |
|-----------------------------------------------------------------------|----------------------------------------------------------------------------------------------------|
| `NewTranslator(opts ...filter.TranslatorOption) (*Translator, error)` | Returns `filter.ErrAllowlistRequired` when `WithUntrustedInput` is set without `WithAllowedFields` |

## Methods

| Method                                   | Description                                                                 |
|------------------------------------------|-----------------------------------------------------------------------------|
| `Translate(node) (string, []any, error)` | Parameterized clause; every literal becomes a `?` and is returned in `args`  |
| `TranslateInline(node) (string, error)`  | Self-contained clause with literals rendered in place                        |

A `nil` node translates to `1 = 1` (match all) in both modes, so `"... WHERE " + where` stays valid without a special case.

## Usage

```go
parser, _ := filter.NewParser()
ast, _ := parser.Parse(ctx, `status == 2 && type in [1, 2]`)

trans, err := mariadb.NewTranslator(
    filter.WithUntrustedInput(),
    filter.WithAllowedFields("status", "type"),
)
if err != nil {
    return err
}

where, args, err := trans.Translate(ast)
// where: (`status` = ?) AND (`type` IN (?, ?))
// args:  []any{int64(2), int64(1), int64(2)}

rows, err := db.QueryContext(ctx, "SELECT * FROM events WHERE "+where, args...)
```

## Operation mapping

| CEL                          | MariaDB                                 |
|------------------------------|-----------------------------------------|
| `==` `!=` `<` `>` `<=` `>=`  | `=` `!=` `<` `>` `<=` `>=`              |
| `&&` `\|\|` `!`              | `AND` `OR` `NOT`                        |
| `field`                      | `` `field` = TRUE ``                    |
| `field in [a, b]`            | `` `field` IN (?, ?) ``                 |
| `field in []`                | `1 = 0`                                 |
| `field == null`              | `` `field` IS NULL ``                   |
| `field != null`              | `` `field` IS NOT NULL ``               |
| `has(field)`                 | `` `field` IS NOT NULL ``               |
| `field.contains(s)`          | `` LOCATE(?, `field`) > 0 ``            |
| `field.startsWith(s)`        | `` LOCATE(?, `field`) = 1 ``            |
| `field.endsWith(s)`          | `` RIGHT(`field`, CHAR_LENGTH(?)) = ? `` |
| `field.matches(re)`          | `` `field` REGEXP ? ``                  |
| `field.size()` / `size(f)`   | `` CHAR_LENGTH(`field`) ``              |
| `substring()`                | `filter.ErrUnsupportedOperation`        |

`contains` and `startsWith` go through `LOCATE`, which takes the needle as a plain string — there is no LIKE pattern, so a `%` or `_` in the
operand stays literal. `startsWith` is `LOCATE = 1` because MariaDB has no dedicated function; the two are equivalent, including for an empty
needle. `size()` is rejected outside a comparison: `CHAR_LENGTH` is an integer expression, and on its own there is no predicate for `WHERE` to
test.

**`endsWith` binds its operand twice.** MariaDB has no `endsWith`, so the needle both sizes the suffix and is compared to it. One CEL argument
consumes two placeholders — `args` reflects that, and any clause following it is numbered accordingly.

## Column names

Field names are checked against `WithAllowedFields`, run through `WithFieldMapping`, validated as plain SQL identifiers, and emitted
backtick-quoted per dot-separated segment: `address.city` becomes `` `address`.`city` ``.

The validation is stricter than backtick quoting requires: the column position is the one part of the generated SQL that no placeholder can
cover, so anything that is not `[A-Za-z_][A-Za-z0-9_]*` (dot-separated) is rejected with `filter.ErrInvalidExpression`. To filter on a JSON
path or a computed value, map the CEL name onto a generated column.

## Collation

String comparison follows the column's collation, and MariaDB's defaults (`utf8mb4_general_ci` and friends) are **case-insensitive**. Every
string predicate here inherits that: `contains`, `startsWith`, `endsWith`, `REGEXP` and plain equality all match case-insensitively unless the
column or the connection says otherwise. This is a real semantic difference from the ClickHouse and PostgreSQL translators, which are
case-sensitive. Use an explicit `_bin` or `_cs` collation where the distinction matters.

## Null semantics

A SQL row always carries every column of its table, so there is no "missing field" for `has()` to detect — it compiles to `IS NOT NULL` and is
meaningful only against a nullable column. The same three-valued logic applies to `!=`: a NULL row does not satisfy `` `col` != 'x' ``, because
the comparison yields NULL rather than true. Write `col != "x" || col == null` where the NULL row should match.

## Regular expressions

`matches()` compiles to `REGEXP`, which MariaDB 10.0.5+ evaluates with PCRE. Unlike RE2 it can backtrack, so a crafted pattern can be made
expensive server-side. `filter.WithMaxRegexLength` bounds the blast radius but does not eliminate it — expose `matches()` to trusted callers,
or tighten the cap.

## Timestamps

`timestamp(...)` literals bind as `time.Time` in the parameterized form and render as a UTC `DATETIME` literal at microsecond precision inline.
Store the corresponding columns in UTC.

## Inline rendering

`TranslateInline` doubles the quote in a string literal, which closes the break-out in **every** `sql_mode`. It also doubles the backslash,
which is correct under the default mode where the backslash is an escape character. Under `NO_BACKSLASH_ESCAPES` it is not, and a value
containing a backslash comes out with it doubled — a fidelity bug, never an injection. Use the parameterized form where exact values matter.

## Concurrency

A `Translator` accumulates per-call state (nesting depth, collected arguments) and is **not** safe for concurrent use. Construct one per
goroutine, or guard it with a mutex.

## See also

- [`data/filter`](../..) — parser, AST, and the shared translator options
- [`translators/postgres`](../postgres), [`translators/clickhouse`](../clickhouse) — the other SQL dialects
