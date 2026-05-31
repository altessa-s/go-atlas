# Order_by (`orderby`)

```go
import "github.com/altessa-s/go-atlas/data/orderby"
```

An [AIP-132](https://google.aip.dev/132#ordering) `order_by` parser with translators for MongoDB, Meilisearch, and RediSearch. One DSL string —
`"create_time desc, slug"` — parses to a flat `Spec` value, then each translator emits the sort representation its backend expects (`bson.D` for
Mongo, `"field:asc"` slices for Meili, single-key `SortBy` for RediSearch). Built with the same defense-in-depth posture as `data/filter`: length
caps, key-count caps, identifier validation, allow-list with wildcards, and an explicit "untrusted input requires allow-list" guard.

---

## When to reach for `orderby`

| Scenario | Use |
|---|---|
| Accept `order_by` from a `List*Request` and sort a MongoDB query | `Parser` + `translators/mongo.NewTranslator` |
| Sort a Meilisearch search request | `translators/meili.NewTranslator` |
| Sort a RediSearch `FT.SEARCH` (single key only) | `translators/redisearch.NewTranslator` |
| Expose API field names that differ from storage columns | `WithFieldMapping` (exact) or `WithFieldPrefixMapping` (subtree) |
| Untrusted `order_by` from end users | `WithUntrustedInput` + `WithAllowedFields` (mandatory pair) |
| Sort on nested documents (`address.city`) | Default — parser already accepts dotted paths |
| Allow whole subtrees without enumerating leaves | `WithAllowedFields("address.*")` |
| Sort on array positions (`tags.0`) | `WithAllowArrayIndexPaths()` (opt-in, off by default) |

---

## Quick start

### `order_by` → MongoDB sort document

```go
parser, err := orderby.NewParser()
if err != nil { return err }

spec, err := parser.Parse(ctx, "create_time desc, slug")
if err != nil { return err }

trans, err := mongo.NewTranslator(
    orderby.WithAllowedFields("create_time", "slug"),
)
if err != nil { return err }

sort, _ := trans.Translate(spec)
// sort == bson.D{{"create_time", int32(-1)}, {"slug", int32(1)}}

cursor, err := col.Find(ctx, filter, options.Find().SetSort(sort))
```

### `order_by` → Meilisearch sort argument

```go
spec, _ := parser.Parse(ctx, "createdAt desc, name")
trans, _ := meili.NewTranslator(orderby.WithAllowedFields("createdAt", "name"))
sort, _ := trans.Translate(spec)
// sort == []string{"createdAt:desc", "name:asc"}

req := &meilisearch.SearchRequest{Sort: sort}
```

### `order_by` → RediSearch single SORTBY

```go
spec, _ := parser.Parse(ctx, "createdAt desc")
trans, _ := redisearch.NewTranslator(orderby.WithAllowedFields("createdAt"))
sb, _ := trans.Translate(spec)
// sb == redisearch.SortBy{Field: "createdAt", Descending: true}
```

Always check the constructor error — the misconfiguration check fires at construction.

### Untrusted input (HTTP query parameter)

```go
trans, err := mongo.NewTranslator(
    orderby.WithUntrustedInput(),
    orderby.WithAllowedFields("createdAt", "slug", "address.*"),
)
if errors.Is(err, orderby.ErrAllowlistRequired) {
    log.Fatal("untrusted-input translator without allow-list is a misconfiguration")
}
```

`WithUntrustedInput` makes the allow-list mandatory — without `WithAllowedFields` the **constructor** refuses to run, not the first `Translate`
call. Misconfiguration surfaces at process start rather than on the first request.

---

## DSL grammar

```
order_by  := key ("," key)*
key       := field_path direction?
field_path:= ident ("." ident)*
direction := "asc" | "desc"
```

- Whitespace around commas and between field and direction is insignificant.
- Direction default is ascending.
- Identifiers match `[A-Za-z_][A-Za-z0-9_]*` per segment and are **case-sensitive** — `Slug` and `slug` are different fields.
- Direction tokens are case-**insensitive** by default (`ASC`/`Asc`/`asc` all work); flip with `WithCaseSensitiveDirection`.
- Empty or whitespace-only input parses to an empty `Spec` with no error — AIP-132's "no ordering specified" semantics.
- Duplicate field paths (`"slug, slug desc"`) return `ErrDuplicateKey` — almost always a client bug, and wire-level semantics differ between
  backends, so the parser surfaces one consistent error regardless of the target.

---

## Security limits

| Option | Default | What it bounds |
|---|---|---|
| `WithMaxExpressionLength` | 1024 bytes | Reject before parsing — protects parser memory and the LRU cache |
| `WithMaxKeys` | 32 | Maximum number of sort keys (commas + 1) |
| `WithMaxFieldPathDepth` | 8 | Maximum dotted segments in one field path |
| `WithMaxFieldNameLength` | 128 bytes | Maximum length of one field path including its dots |
| `WithParserCacheSize` | 1000 entries | LRU cache for parsed `Spec` values |
| `WithParserNoCache` | -- | Disable caching entirely (typical for tests) |
| `WithCaseSensitiveDirection` | off | Reject `DESC`/`Asc` — only lowercase tokens accepted |
| `WithAllowArrayIndexPaths` | off | Relax grammar to accept all-digit segments (`tags.0`) |
| `WithParserCollector` | -- | Prometheus metrics collector for parse latency and error counters |

Length and key caps belong on the parser (`ParserOption`); allow-list and mapping belong on the translator (`TranslatorOption`). Order_by strings
are short by design — defaults are well above any realistic production payload, meant only as a safety stop against pathologically large input.

---

## Translator options

| Option | Default | Description |
|---|---|---|
| `WithAllowedFields(fields...)` | all | Whitelist of sortable field names. Exact match by default; entries ending in `.*` match every subpath; a lone `*` matches anything |
| `WithFieldMapping(map)` | identity | Exact DSL-name → DB-column mapping |
| `WithFieldPrefixMapping(map)` | -- | Subtree rewrite — `"address." → "addr."`. Keys/values must end in `.`; longest prefix wins; exact mapping overrides |
| `WithUntrustedInput()` | off | Marks input as user-supplied; refuses to construct without a non-empty allow-list |

Allow-list keys match the **DSL** name (`Key.Name`), not the mapped DB column. Pair `WithFieldMapping("createdAt" → "created_at")` with
`WithAllowedFields("createdAt")`, not `…("created_at")` — otherwise every sort is rejected.

---

## Nested paths

Dotted field paths (`address.city`, `user.profile.name`) are accepted by the parser by default and emitted verbatim by all three translators —
MongoDB, Meilisearch, and RediSearch all support them natively. Three knobs make working with nested schemas less verbose.

### Wildcard allow-list

`WithAllowedFields` accepts entries that end in `.*` as "any subtree under this prefix":

```go
trans, _ := mongo.NewTranslator(
    orderby.WithAllowedFields(
        "createdAt",     // exact
        "slug",          // exact
        "address.*",     // address.city, address.zip.code, but NOT bare "address"
    ),
)
```

| Pattern | Matches `address.city`? | Matches `address`? | Matches `billing.city`? |
|---|---|---|---|
| `address` (exact) | no | yes | no |
| `address.*` (wildcard) | yes | no | no |
| `address` + `address.*` | yes | yes | no |
| `*` (bare star) | yes | yes | yes |

Embedded asterisks (`foo.*.bar`) have no special meaning — they are stored as exact strings and will never match a real key. Wildcards are
sorted longest-first internally, so `address.deep.*` correctly precedes `address.*` when both are configured.

### Prefix field-mapping

`WithFieldPrefixMapping` rewrites the leading segments of dotted paths. Each key and value must end in `.` so the rewrite happens on a segment
boundary:

```go
trans, _ := mongo.NewTranslator(
    orderby.WithFieldPrefixMapping(map[string]string{
        "address.":      "addr.",       // address.city.zip → addr.city.zip
        "user.profile.": "users.prof.", // longest-match wins over "user."
    }),
)
```

Precedence: exact `WithFieldMapping` entries always win over any prefix rewrite. Multiple prefix matches resolve to the longest. Entries whose
key or value does not end in `.` are silently dropped — without the trailing dot a key like `addr` would corrupt unrelated identifiers
(`address` → `adress`).

### Array-index segments

Strict AIP-132 grammar rejects all-digit segments. Opt in at the parser:

```go
parser, _ := orderby.NewParser(orderby.WithAllowArrayIndexPaths())
spec, _ := parser.Parse(ctx, "tags.0 desc, items.5.name")
```

Numeric segments accept leading zeros (`tags.001`) — Mongo and JSONPath both do. Whether the backend has the corresponding index is the caller's
concern; `orderby` validates the syntax only.

---

## Translators

| Package | Output | Empty input | Notes |
|---|---|---|---|
| `translators/mongo` | `bson.D` (ordered) | `nil` | `bson.D` preserves sort precedence; `bson.M` does not |
| `translators/meili` | `[]string` (`"field:asc"`) | `nil` | Mirrors `data/filter/translators/meili` |
| `translators/redisearch` | `SortBy{Field, Descending}` | zero-value `SortBy{}` | RediSearch `FT.SEARCH … SORTBY` is single-key — multi-key input returns `ErrTooManySortKeys` |

All three constructors share the signature `NewTranslator(opts ...orderby.TranslatorOption) (*Translator, error)`. The constructor validates
`WithUntrustedInput` + `WithAllowedFields` consistency once; the per-`Translate` hot path never reruns the check.

### MongoDB

```go
trans, _ := mongo.NewTranslator(
    orderby.WithAllowedFields("createdAt", "slug"),
    orderby.WithFieldMapping(map[string]string{"createdAt": "created_at"}),
)

spec, _ := parser.Parse(ctx, "createdAt desc, slug")
sort, _ := trans.Translate(spec)
// sort == bson.D{{"created_at", int32(-1)}, {"slug", int32(1)}}
```

Ascending keys emit `int32(1)`, descending emit `int32(-1)`. Empty input returns `nil` — `SetSort(nil)` is a valid driver no-op.

### Meilisearch

```go
trans, _ := meili.NewTranslator(orderby.WithAllowedFields("createdAt", "name"))
spec, _ := parser.Parse(ctx, "createdAt desc, name")
sort, _ := trans.Translate(spec)
// sort == []string{"createdAt:desc", "name:asc"}
```

Empty input returns `nil`. The output matches Meilisearch's `sort` argument shape directly.

### RediSearch

```go
trans, _ := redisearch.NewTranslator(orderby.WithAllowedFields("createdAt"))
spec, _ := parser.Parse(ctx, "createdAt desc")
sb, _ := trans.Translate(spec)
// sb == redisearch.SortBy{Field: "createdAt", Descending: true}
```

Empty input returns zero-value `SortBy{}` (`Field == ""`). Multi-key input returns `ErrTooManySortKeys` — RediSearch `FT.SEARCH … SORTBY` takes
one field. Whether the index declares the field as `SORTABLE` is the caller's responsibility.

---

## Diagnostics

All errors are sentinels wrapped via `core/errors.Wrapf` for contextual detail. Use `errors.Is` for classification.

| Sentinel | When it fires |
|---|---|
| `ErrParseFailed` | Clause cannot be tokenised into `field [asc\|desc]` |
| `ErrEmptyClause` | Clause between commas is empty (`",a"`, `"a,,b"`) |
| `ErrInvalidFieldPath` | Field path contains a non-ident character or empty segment |
| `ErrInvalidDirection` | Direction token other than `asc`/`desc` (case-sensitive when configured) |
| `ErrExpressionTooLong` | Raw input exceeds `MaxExpressionLength` |
| `ErrMaxKeysExceeded` | More clauses than `MaxKeys` |
| `ErrMaxFieldPathDepthExceeded` | Dotted segments exceed `MaxFieldPathDepth` |
| `ErrMaxFieldNameLengthExceeded` | Field-path length exceeds `MaxFieldNameLength` |
| `ErrDuplicateKey` | Same field path appears twice |
| `ErrFieldNotAllowed` | Field absent from `WithAllowedFields` (after wildcard scan) |
| `ErrAllowlistRequired` | `WithUntrustedInput` configured without a non-empty allow-list (raised at `NewTranslator`/`NewTranslatorContext`) |
| `ErrTooManySortKeys` | RediSearch translator received >1 key |

Error precedence inside `Parse` is deterministic: length cap → key count cap → per-clause syntax (`ErrEmptyClause`, `ErrParseFailed`,
`ErrInvalidFieldPath`, `ErrInvalidDirection`, `ErrMaxFieldPathDepthExceeded`, `ErrMaxFieldNameLengthExceeded`) → `ErrDuplicateKey`. A malformed
duplicate (`"slug, slug upward"`) reports its syntactic problem first, not the duplicate.

---

## Performance

Benchmarks on Apple M4 Pro (data may vary by host; bench source lives next to each translator).

| Operation | ns/op | allocs/op |
|---|---|---|
| `Parse` cache hit (5 keys) | ~50 | 2 |
| `Parse` no-cache (5 keys, fresh) | ~370 | 13 |
| `Parse` no-cache (1 key) | ~170 | 4 |
| Mongo `Translate` (1 key, no options) | ~35 | 2 |
| Mongo `Translate` (5 keys, no options) | ~92 | 3 |
| Mongo `Translate` (3 keys, wildcard allow-list of 5) | ~104 | 3 |
| Mongo `Translate` (3 keys, prefix-mapping of 5) | ~149 | 6 |
| Meili `Translate` (1 key) | ~50 | 2 |
| Meili `Translate` (5 keys) | ~205 | 6 |
| RediSearch `Translate` (1 key) | ~5 | 0 |

The wildcard scan and prefix-mapping scan add O(P) overhead per key, where P is the configured prefix count. For typical P ≤ 10 the overhead is
~20-30 ns/key.

`Parse` returns a deep-copied `Spec` on every call — callers can freely mutate `Keys` without corrupting a subsequent cache hit. The copy is one
slice allocation per call (Key fields are value types, no inner slices).

---

## Caching

The parser keeps an LRU of `Spec` values keyed by the raw input string. Both successful parses and parse errors are cached, so repeated
malformed input does not re-pay the validation cost. Default capacity is 1000; disable with `WithParserNoCache()` (typical for tests, never for
production).

`parseDuration` Prometheus histogram covers both hits and misses — the operator-relevant latency. Cache hits show up as the fast P50/P90; misses
sit in the long tail. A growing tail with a healthy hit rate signals a workload shift, not a regression.

---

## Best practices

- **Construct once, reuse.** `*Parser` and each translator are safe for concurrent use; build them at startup, share across handlers.
- **Configure on the right side.** Resource caps belong on the parser; allow-list and mapping belong on the translator. Sharing options between
  the two is intentional only for `WithParserCollector` (metrics).
- **Untrusted input always needs an allow-list.** The construction-time guard is your last line; treat `ErrAllowlistRequired` as a fail-fast
  signal, not a runtime branch.
- **Wildcards are a convenience, not a security feature.** A wildcard `address.*` still indexes through Mongo dotted notation — make sure the
  backend index actually supports the depth you advertise.
- **Allow-list keys are DSL names, not column names.** With `WithFieldMapping("createdAt" → "created_at")`, write
  `WithAllowedFields("createdAt")`.
- **Don't sneak array indices into APIs by default.** `WithAllowArrayIndexPaths` is opt-in for a reason — `tags.0` is rarely a stable, indexed
  sort target.
- **Don't try to stretch defaults.** If you genuinely need 50 sort keys or 12-deep field paths, your API design is leaking storage shape.

---

## See also

- [Package README](../../data/orderby/README.md) — Go-doc-style quick reference.
- [Filter (`filter`)](filter.md) — sibling package for the AIP-160 filter DSL with the same security model.
- [AIP-132](https://google.aip.dev/132) — Google's API Improvement Proposal for ordering and pagination.
