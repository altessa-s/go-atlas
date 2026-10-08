# projection

```go
import "github.com/altessa-s/go-atlas/data/projection"
```

Package `projection` parses a client-supplied field list — the AIP-157 partial-response `fields` parameter — and translates it into a
database-specific projection, so unrequested fields are never read from storage instead of being trimmed off the response afterwards. It is the
field-selection counterpart of [`data/orderby`](../orderby): a cached parser produces a `Spec`, and per-backend translators turn it into a MongoDB
projection document or a SQL column list under one shared policy of allowed, denied, mapped and required fields.

## Grammar

```
fields := "" | "*" | path ("," path)*
path   := ident ("." ident)*
ident  := [A-Za-z_][A-Za-z0-9_]*
```

A restricted subset of the [AIP-161](https://google.aip.dev/161) field mask grammar: a value from `x-goog-fieldmask`, `$fields` or
`fieldmaskpb.FieldMask.Paths` parses as long as it stays inside the subset. Whitespace around commas is insignificant; identifiers are
case-sensitive. Empty, whitespace-only and `*` input select **all authorized fields**. Duplicates are dropped and the paths sorted; a parent and its
child are both kept, because collapsing them is only safe after field mapping.

| Input                             | Result                                     |
|-----------------------------------|--------------------------------------------|
| `name, address.city`              | `Spec{Paths: [address.city name]}`         |
| `""`, `"*"`                       | empty `Spec` — the default selection       |
| `a,,b`                            | `ErrEmptyClause`                           |
| `$where`, `a..b`, `1a`            | `ErrInvalidFieldPath`                      |
| `items.*.name`, `a,*`, `items.0`, `` labels.`k` `` | `ErrUnsupportedPath`      |

## Key types

| Type                | Description                                                                          |
|---------------------|--------------------------------------------------------------------------------------|
| `Parser`            | Fields parser with LRU caching: `Parse(ctx, string)`, `ParsePaths(ctx, []string)`    |
| `Spec`              | Parsed request: sorted, unique API paths; empty means "all authorized fields". `Relative(prefix)` re-roots an envelope mask |
| `Selection`         | Resolved projection in storage names: `Include` or `Exclude`, or neither for "all"   |
| `TranslatorContext` | Shared policy; `Resolve(Spec)` and `DefaultSelection()` for backend translators      |

## Parser options

| Option                    | Default | Description                                                              |
|---------------------------|---------|--------------------------------------------------------------------------|
| `WithParserCacheSize`     | 1000    | LRU cache capacity for parsed `Spec` values                              |
| `WithParserNoCache`       | --      | Disable caching                                                          |
| `WithMaxExpressionLength` | 4096    | Maximum raw input length in bytes (joined length for `ParsePaths`)       |
| `WithMaxPaths`            | 64      | Maximum number of paths, counted before deduplication                    |
| `WithMaxFieldPathDepth`   | 8       | Maximum number of dot-separated segments in one path                     |
| `WithMaxFieldNameLength`  | 128     | Maximum length of one path in bytes                                      |
| `WithParserCollector`     | --      | Metrics collector for parse latency and error counters                   |

## Translator options

| Option                    | Default  | Description                                                                                       |
|---------------------------|----------|---------------------------------------------------------------------------------------------------|
| `WithAllowedFields`       | all      | Requestable API paths. `address.*` allows the subtree but **not** `address`; a lone `*` allows all |
| `WithFieldMapping`        | identity | Exact API-name → storage-name mapping                                                             |
| `WithFieldPrefixMapping`  | --       | Rewrite leading segments (`"address." → "addr."`); longest prefix wins; exact mapping overrides    |
| `WithDeniedFields`        | --       | API paths never returned; a request equal to, under, or above one fails                           |
| `WithDeniedStorageFields` | --       | Storage paths never returned, whatever API name maps onto them                                    |
| `WithRequiredFields`      | --       | Storage paths added to every inclusion projection (`_id`, tenant, version, keyset sort keys)       |
| `WithDefaultFields`       | derived  | API paths returned for an empty request                                                           |
| `WithUntrustedInput`      | --       | Require a non-empty allow-list, otherwise `ErrAllowlistRequired` at construction                  |

Every request path is checked against the allow-list and `WithDeniedFields` by API name, then mapped and checked against the denied storage
names — `WithDeniedStorageFields` plus every storage path a denied API field reaches through the mapping (its own name, the targets its
descendants are mapped to, the names an exact ancestor entry implies) — so an allowed alias cannot reach a protected column.

A path selects its whole subtree in storage: `address` with prefix mapping `"address." → "addr."` selects `addr`, and a descendant mapped
elsewhere (`"address.city" → "city_col"`) is selected alongside it. The same rule turns the wildcard root `address.*` into a default selection.

### Empty requests

"All fields" never bypasses the policy. The default selection is derived once, at construction:

| Policy                                  | Default selection                                                   |
|-----------------------------------------|---------------------------------------------------------------------|
| `WithDefaultFields(...)`                | those paths + required fields                                       |
| Restrictive allow-list                  | the allow-list roots (`address.*` → `address`) + required fields     |
| No allow-list (or `*`), denied fields   | every field except the denied ones (exclusion; MongoDB only)        |
| No allow-list, nothing denied           | every field                                                         |

A policy whose default cannot be derived safely — an empty allow-list, an allow-list root that carries a denied field, or a SQL backend with denied
fields and no default — fails with `ErrDefaultFieldsRequired`. A required field that overlaps a denied one, or a default field outside the
allow-list or overlapping a denied one, fails with `ErrConflictingPolicy`.

## Usage

```go
parser, _ := projection.NewParser()
spec, err := parser.Parse(ctx, req.GetFields())
if err != nil {
    return status.Error(codes.InvalidArgument, err.Error())
}

trans, err := mongo.NewTranslator(
    projection.WithUntrustedInput(),
    projection.WithAllowedFields("name", "email", "createTime", "address.*"),
    projection.WithFieldMapping(map[string]string{"createTime": "created_at"}),
    projection.WithDeniedStorageFields(deniedFromBehaviorTags...),
    projection.WithRequiredFields("_id", "created_at"),
)
proj, err := trans.Translate(spec)

res, err := datamongo.ListCursor[User](ctx, col,
    datamongo.WithListCursorSort(bson.D{{Key: "created_at", Value: -1}}),
    datamongo.WithListCursorProjection(proj),
)
```

The `fields` value can also come from the AIP-157 read mask captured by the gRPC
[`fieldmask` interceptor](../../transport/grpc/interceptors/fieldmask):

```go
if mask, ok := grpcfieldmask.ReadMaskFromContext(ctx); ok {
    spec, err = parser.ParsePaths(ctx, mask.GetPaths())
    // A List* mask addresses the response envelope ("items.name, next_page_token"):
    spec, _ = spec.Relative("items")
}
```

The policy for a behavior-tagged model can be derived with
[`domain/behavior/translators/mongo.NewFieldPathsTranslator`](../../domain/behavior/translators/mongo): its `Denied` paths feed
`WithDeniedStorageFields`, its `Selectable` paths an allow-list.

## Pagination

Keyset pagination reads the cursor and sort values back from the last returned row. `data/mongo.ListCursor` rejects a projection that drops the
cursor ID field or the primary sort field (`ErrProjectionDropsCursorField`); list them with `WithRequiredFields`.

## Subpackages

| Package                                          | Output                         | Empty request with no policy |
|--------------------------------------------------|--------------------------------|------------------------------|
| [translators/mongo](./translators/mongo)         | `bson.M` (`{p: 1}` / `{p: 0}`) | `nil`                        |
| [translators/postgres](./translators/postgres)   | `string` (`"a", "b"`)          | `""` → `*`                   |
| [translators/mariadb](./translators/mariadb)     | `string` (`` `a`, `b` ``)      | `""` → `*`                   |
| [translators/clickhouse](./translators/clickhouse) | `string` (`` `a`, `b` ``)    | `""` → `*`                   |

## Errors

| Error                           | Meaning                                                                  |
|---------------------------------|--------------------------------------------------------------------------|
| `ErrEmptyClause`                | Empty path between commas                                                |
| `ErrInvalidFieldPath`           | Path outside the grammar, or a storage name a backend cannot address     |
| `ErrUnsupportedPath`            | Valid AIP-161 syntax this package does not serve (wildcards, keys, indexes) |
| `ErrExpressionTooLong`          | Input longer than `MaxExpressionLength`                                  |
| `ErrMaxPathsExceeded`           | More than `MaxPaths` paths                                               |
| `ErrMaxFieldPathDepthExceeded`  | Path deeper than `MaxFieldPathDepth`                                     |
| `ErrMaxFieldNameLengthExceeded` | Path longer than `MaxFieldNameLength`                                    |
| `ErrFieldNotAllowed`            | Path outside the allow-list or overlapping a denied field                |
| `ErrAllowlistRequired`          | `WithUntrustedInput` without a non-empty allow-list                      |
| `ErrConflictingPolicy`          | Required or default field contradicts the deny-list or allow-list        |
| `ErrDefaultFieldsRequired`      | The empty-request projection cannot be derived; set `WithDefaultFields`  |
