# postgres

```go
import "github.com/altessa-s/go-atlas/data/projection/translators/postgres"
```

Translates a [`projection.Spec`](../../README.md) into the column list of a PostgreSQL `SELECT` — quoted columns joined by `, `, without the
keyword. The companion of [`data/orderby/translators/postgres`](../../../orderby/translators/postgres).

## Output

| Request / policy                                 | Output                                              |
|--------------------------------------------------|-----------------------------------------------------|
| Non-empty request                                | requested + required columns, lexical order         |
| Empty request, `WithDefaultFields` or allow-list | default + required columns                          |
| Empty request, no policy                         | `""` — the caller writes `*`                        |
| Empty request, denied fields, no default         | `NewTranslator` fails with `ErrDefaultFieldsRequired` |

Every storage name, after field mapping, must be `col` or `table.col`; anything else is rejected with `projection.ErrInvalidFieldPath`.
Columns carry no aliases, so scan rows by column name.

## Usage

```go
trans, err := postgres.NewTranslator(
    projection.WithUntrustedInput(),
    projection.WithAllowedFields("name", "email"),
    projection.WithRequiredFields("id"),
)
cols, err := trans.Translate(spec) // "id", "name"
query := "SELECT " + cmp.Or(cols, "*") + " FROM users WHERE …"
```
