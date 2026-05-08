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
