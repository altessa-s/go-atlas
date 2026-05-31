# orderby translators

Each subpackage converts an [`orderby.Spec`](..) value into the sort representation expected by a specific storage backend. All translators share
the same `orderby.TranslatorContext` (allow-list, field mapping, untrusted-input guard) so security knobs configured on one translator transfer
verbatim to another. The misconfiguration check fires at `NewTranslator` construction — a successfully constructed translator is always allowed
to translate.

| Package        | Output                       | Empty input       | Notes                                                                 |
|----------------|------------------------------|-------------------|-----------------------------------------------------------------------|
| [mongo](mongo) | `bson.D`                     | `nil`             | Ordered document — Mongo respects insertion order on `bson.D` only    |
| [meili](meili) | `[]string` (`"field:asc"`)   | `nil`             | Mirrors `data/filter/translators/meili`                               |
| [redisearch](redisearch) | `SortBy{Field, Descending}` | `SortBy{}` (`Field == ""`) | `FT.SEARCH ... SORTBY` accepts a single field; multi-key input is rejected |

Empty-input outputs are deliberately uniform where the Go type allows: slice-returning translators (Mongo, Meili) yield `nil`, so the call site
can use a single `if sort != nil { … }` check across both. RediSearch returns a value-type `SortBy` (no `nil`), and callers test
`sortBy.Field == ""` instead.
