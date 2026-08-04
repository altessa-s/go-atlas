# meili

```go
import "github.com/altessa-s/go-atlas/data/filter/translators/meili"
```

Package `meili` translates filter AST nodes into Meilisearch filter expressions
(strings such as `(status = 2) AND (type IN [1, 2])`). Supports comparisons,
logical operators, list membership, `contains` / `startsWith` predicates and
`has()` field-existence checks. Operations without a Meilisearch counterpart —
`endsWith`, `matches` (regex), `size()` — are rejected with
`filter.ErrUnsupportedOperation`.

`timestamp(...)` literals are emitted as Unix seconds (Meilisearch filters
numeric attributes only); store the corresponding fields as numeric epoch
seconds and add them to the index's `filterableAttributes`. Sub-second
precision is dropped.

## Null

Meilisearch splits a question CEL's `null` treats as one. `EXISTS` asks whether the attribute is present in the document; `IS NULL` asks whether
it is present **and** null. A filter built from `IS NULL` alone would miss every document that simply omits the attribute — and its negation
would match all of them, which is how a soft-delete filter comes to return the deleted documents.

Both halves are therefore covered explicitly:

| CEL              | Meilisearch                                        |
|------------------|----------------------------------------------------|
| `field == null`  | `(field IS NULL OR field NOT EXISTS)`              |
| `field != null`  | `(field EXISTS AND field IS NOT NULL)`             |
| `has(field)`     | `field EXISTS`                                     |

## Bare identifiers

A bare identifier used as a condition becomes a boolean test — `active` translates to `active = true` — at the root of an expression and on
either side of `AND` / `OR` alike. `CONTAINS` and `STARTS WITH` are gated behind Meilisearch's `containsFilter` experimental feature; enable it
on the server before using `contains()` or `startsWith()`.
