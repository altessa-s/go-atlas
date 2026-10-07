# postgres

```go
import "github.com/altessa-s/go-atlas/data/orderby/translators/postgres"
```

Translates an [`orderby.Spec`](../..) into the body of a PostgreSQL `ORDER BY` clause — the companion of
[`data/filter/translators/postgres`](../../../filter/translators/postgres).

## Key types

| Symbol                     | Description                                                       |
|----------------------------|-------------------------------------------------------------------|
| `NewTranslator(opts...)`   | Builds a translator from the shared `orderby.TranslatorOption`s   |
| `Translator.Translate(ob)` | Returns `col DIR, …` without the keywords; `""` for an empty spec |

## Semantics

| Aspect      | Behavior                                                                                                                 |
|-------------|--------------------------------------------------------------------------------------------------------------------------|
| Output      | `"created_at" DESC, "slug" ASC` for `create_time desc, slug` mapped to `created_at`                                      |
| Identifiers | After mapping, a column or `table.column` of plain identifiers, each part quoted; anything else is `ErrInvalidFieldPath` |
| NULLs       | No `NULLS FIRST/LAST`: NULL sorts as the largest value: last in `ASC`, first in `DESC`                                   |
| Errors      | `ErrFieldNotAllowed`, `ErrInvalidFieldPath`, `ErrAllowlistRequired`                                                      |

A sort column cannot be bound as a parameter, so mapped names are validated rather than spliced in: map a nested field onto a generated column to
sort by it, and add a unique column last for a deterministic order.

## Usage

```go
trans, err := postgres.NewTranslator(orderby.WithAllowedFields("createdAt", "slug"), orderby.WithUntrustedInput(),
	orderby.WithFieldMapping(map[string]string{"createdAt": "created_at"}))
clause, err := trans.Translate(ob)
if clause != "" {
	query += " ORDER BY " + clause
}
```
