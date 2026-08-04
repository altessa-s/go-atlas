# sqlbase

```go
import "github.com/altessa-s/go-atlas/data/filter/translators/internal/sqlbase"
```

Internal package. Holds the AST walk shared by the SQL filter translators — [`clickhouse`](../../clickhouse), [`mariadb`](../../mariadb) and
[`postgres`](../../postgres). Not importable outside the module.

## Why it exists

The three SQL translators differ in exactly four places:

| Difference          | ClickHouse                  | MariaDB                            | PostgreSQL                            |
|---------------------|-----------------------------|------------------------------------|---------------------------------------|
| Identifier quoting  | `` `address.city` ``        | `` `address`.`city` ``             | `"address"."city"`                    |
| Bind marker         | `?`                         | `?`                                | `$1`, `$2`, …                         |
| String predicates   | `position` / `startsWith`   | `LOCATE` / `RIGHT` / `REGEXP`      | `strpos` / `starts_with` / `right` / `~` |
| Inline literals     | `\`-escaped, `unhex()`      | `''`-doubled, `X'…'`               | `E'…'`, `decode(…, 'hex')`            |

Everything else is identical: the traversal, the allow-list and field-type checks, depth accounting, argument collection, IN lists, null
handling and the comparison operators. That part lives here; the four differences are supplied by a `Dialect`.

## Key types

| Type / Function                                                   | Description                                                                 |
|-------------------------------------------------------------------|-------------------------------------------------------------------------------|
| `Translator`                                                      | The shared walker; implements `filter.Visitor`                               |
| `New(dialect, opts ...filter.TranslatorOption) (*Translator, error)` | Constructor; validates the untrusted-input/allowlist pairing                |
| `Dialect`                                                         | The five per-backend rendering hooks                                         |
| `ValueFunc`                                                       | Renders a value in the active mode; handed to the dialect so it can bind twice |
| `MatchAll` / `MatchNone`                                          | `1 = 1` / `1 = 0`                                                            |

## Adding a dialect

A public translator package declares a stateless dialect and wraps the walker:

```go
type Translator struct {
    *sqlbase.Translator
}

func NewTranslator(opts ...filter.TranslatorOption) (*Translator, error) {
    base, err := sqlbase.New(dialect{}, opts...)
    if err != nil {
        return nil, err
    }
    return &Translator{Translator: base}, nil
}
```

`Translate` and `TranslateInline` are promoted from the embedded walker, so the public packages carry no translation logic of their own — only
their dialect and its documentation.

## Identifiers

| Function                        | Produces                    | Used by             |
|---------------------------------|-----------------------------|---------------------|
| `ValidateIdent(name)`           | error or nil                | both quoters        |
| `QuoteWhole(name, quote)`       | `` `address.city` ``        | ClickHouse          |
| `QuoteQualified(name, quote)`   | `` `address`.`city` ``      | MariaDB, PostgreSQL |

`ValidateIdent` accepts only `[A-Za-z_][A-Za-z0-9_]*` segments joined by dots. That is stricter than quoting requires, on purpose: the column
position is the one part of a generated clause that no placeholder can cover, so a name that is not a plain identifier is rejected outright
rather than escaped. Because a dialect's quote character cannot survive the check, the quoters need no escaping at all.

The dot policy is the dialect's to choose. `QuoteQualified` produces the standard SQL `"table"."column"`; `QuoteWhole` produces the single
quoted name ClickHouse needs, where `address.city` is one Nested column rather than two identifiers.

## The `ValueFunc` hook

`Dialect.StringPredicate` receives the needle raw plus a `ValueFunc`, rather than a pre-rendered argument, because `endsWith` has no native
form in MariaDB or PostgreSQL. It compiles to `right(col, length(x)) = x`, where the operand appears twice and must be bound twice. Calling
`value` once per occurrence keeps the argument slice aligned with the emitted text — which matters most for PostgreSQL, where the placeholders
are numbered.
