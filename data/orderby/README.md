# orderby

```go
import "github.com/altessa-s/go-atlas/data/orderby"
```

Package `orderby` parses the AIP-132 [`order_by`](https://google.aip.dev/132#ordering) DSL into a flat AST and translates it to database-specific
sort representations via small per-backend translators. Includes the same defense-in-depth controls as `data/filter`: length caps, key count caps,
field allow-lists, and an explicit "untrusted input requires allow-list" guard.

## Grammar

```
order_by  := key ("," key)*
key       := field_path direction?
field_path:= ident ("." ident)*
direction := "asc" | "desc"
```

Whitespace around commas and between the field and direction tokens is insignificant. The default direction is ascending. Empty or whitespace-only
input parses to an empty `Spec` (no error) — AIP-132's "no ordering specified" semantics. Identifiers are case-sensitive. The parser rejects
inputs that mention the same field path twice (`ErrDuplicateKey`) — `"slug, slug desc"` is almost always a client bug, and the wire-level
semantics differ between backends, so flagging it at parse time gives one consistent error regardless of the target.

## Key types

| Type / Interface | Description                                                       |
|------------------|-------------------------------------------------------------------|
| `Parser`         | Order_by parser with LRU caching                                  |
| `Spec`           | Parsed result — flat slice of `Key` values                        |
| `Key`            | Single sort entry: `Name` (raw dotted form) and `Direction`       |
| `Direction`      | `DirectionAscending` (default) or `DirectionDescending`           |
| `TranslatorContext` / `TranslatorOption` | Shared configuration for the per-backend translators |

## Parser options

| Option                       | Default | Description                                                                              |
|------------------------------|---------|------------------------------------------------------------------------------------------|
| `WithParserCacheSize`        | 1000    | LRU cache capacity for parsed `Spec` values                                              |
| `WithParserNoCache`          | --      | Disable expression caching                                                               |
| `WithMaxExpressionLength`    | 1024    | Maximum raw input length in bytes; longer expressions are rejected before parsing        |
| `WithMaxKeys`                | 32      | Maximum number of sort keys                                                              |
| `WithMaxFieldPathDepth`      | 8       | Maximum number of dot-separated segments in a single field path                          |
| `WithMaxFieldNameLength`     | 128     | Maximum length of a single field path in bytes                                           |
| `WithCaseSensitiveDirection` | --      | Reject `DESC`/`Asc` — only the lowercase `asc`/`desc` tokens are accepted                |
| `WithAllowArrayIndexPaths`   | --      | Relax the grammar so all-digit segments are accepted (`tags.0`, `items.5.name`)          |
| `WithParserCollector`        | --      | Prometheus metrics collector for parse latency and error counters                        |

## Translator options

| Option                 | Default  | Description                                                                                    |
|------------------------|----------|------------------------------------------------------------------------------------------------|
| `WithAllowedFields`       | all      | Whitelist of sortable field names. Exact match by default; entries ending in `.*` match every subpath but **not the bare parent** (`address.*` permits `address.city` and `address.zip.code`, not `address` — list both if needed). A lone `*` matches anything. |
| `WithFieldMapping`        | identity | Exact DSL-name → DB-column mapping                                                          |
| `WithFieldPrefixMapping`  | --       | Rewrite the leading segment(s) of dotted paths (`"address." → "addr."`). Keys/values must end in `.`; longest prefix wins; exact `WithFieldMapping` overrides. |
| `WithUntrustedInput`      | --       | Mark translator as receiving untrusted input — requires a non-empty allow-list, otherwise `NewTranslator` returns `ErrAllowlistRequired` |

> **Note.** The allow-list keys are matched against the **DSL** name (`Key.Name`), not the mapped DB column. Pair
> `WithFieldMapping("createdAt" → "created_at")` with `WithAllowedFields("createdAt")`, not `…("created_at")` — otherwise every sort is rejected.

### Nested paths

Dotted field paths (`address.city`, `user.profile.name`) are accepted by the parser by default and emitted verbatim by all three translators —
MongoDB, Meilisearch, and RediSearch all support them natively. Three knobs make working with nested schemas less verbose:

```go
trans, err := mongo.NewTranslator(
    // Allow any subkey of address.* without listing each leaf.
    // Note: "address.*" matches subpaths only — list "address" too
    // if the bare parent should also be sortable.
    orderby.WithAllowedFields("createdAt", "address", "address.*"),

    // Rewrite the leading segment(s) of dotted paths once; deep paths
    // pass through. address.zip.code → addr.zip.code.
    orderby.WithFieldPrefixMapping(map[string]string{"address.": "addr."}),
)
```

Array-index segments (`tags.0`, `items.5.name`) are **rejected by default** to stay strict-AIP-132. Opt in at the parser:

```go
parser, _ := orderby.NewParser(orderby.WithAllowArrayIndexPaths())
ob, _ := parser.Parse(ctx, "tags.0 desc")
```

Whether the backend actually has that index is the caller's concern — `orderby` only validates the syntax.

### Untrusted input

When translating order_by strings from external clients, pair `WithUntrustedInput()` with `WithAllowedFields(...)`. Without an allow-list a hostile
client can sort on any indexed field, so the combination "untrusted + no allow-list" is treated as misconfiguration — `Translate` returns
`ErrAllowlistRequired` instead of proceeding.

```go
trans, err := mongo.NewTranslator(
    orderby.WithUntrustedInput(),
    orderby.WithAllowedFields("createdAt", "slug"),
)
```

## Usage

```go
parser, _ := orderby.NewParser()

ob, err := parser.Parse(ctx, "create_time desc, slug")
if err != nil {
    return err
}

trans, err := mongo.NewTranslator()
if err != nil {
    return err
}
sort, _ := trans.Translate(ob)
// sort == bson.D{{"create_time", -1}, {"slug", 1}}

cursor, err := col.Find(ctx, filter, options.Find().SetSort(sort))
```

## Subpackages

| Package                                              | Output                       | Constructor                                              |
|------------------------------------------------------|------------------------------|----------------------------------------------------------|
| [translators/mongo](./translators/mongo)             | `bson.D`                     | `NewTranslator(opts ...orderby.TranslatorOption)`        |
| [translators/meili](./translators/meili)             | `[]string` (`field:asc`)     | `NewTranslator(opts ...orderby.TranslatorOption)`        |
| [translators/redisearch](./translators/redisearch)   | `SortBy{Field, Descending}`  | `NewTranslator(opts ...orderby.TranslatorOption)`        |

The MongoDB translator emits `bson.D` (not `bson.M`) because sort precedence is significant — the MongoDB driver respects insertion order only on
ordered documents. RediSearch's `FT.SEARCH ... SORTBY` accepts a single field; multi-key inputs return `ErrTooManySortKeys`.
