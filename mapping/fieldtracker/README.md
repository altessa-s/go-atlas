# fieldtracker

```go
import "github.com/altessa-s/go-atlas/mapping/fieldtracker"
```

Package `fieldtracker` detects changed fields between two struct instances — one-shot with `GetChangedFields`, or through a reusable `Tracker` from
`NewTracker`. Field paths are returned in dot notation (e.g. `"address.city"`) using names derived from struct tags or PascalCase-to-snake_case
conversion. Nested structs, embedded structs, slices, and maps are traversed recursively.

Embedded structs are flattened unless the selected tag names them explicitly; `"-"` excludes a field, including an embedded struct. A change in an
embedded pointer's nil state or an opaque embedded value such as `time.Time` is reported under that field's own name. Ignoring a collection element
such as `items[0]` or `labels[key]` also excludes its nested fields. Unicode field names retain valid UTF-8 in generated paths.

## Options

| Option             | Description                                        |
|--------------------|----------------------------------------------------|
| `WithIgnoreFields` | Field paths to skip (dot notation)                 |
| `WithMaxDepth`     | Maximum nesting depth (default `10`, `0` = off)    |
| `WithTagName`      | Struct tag for field names (default `"json"`)       |

The root struct is at depth 0. The limit controls descent into nested structs
and collections, including nested collection elements. Repeated pointer,
map, and slice pairs on the current path end traversal of that branch even
when the depth limit is disabled.

## Paths

Literal field names escape dots, brackets, and backslashes with a backslash.
A field tagged `json:"meta.name"` produces `meta\.name`. A nested field
`name` inside `meta` still produces `meta.name`. Use the escaped form for
ignore rules that target a literal field name.

## Concurrency

`Tracker` is safe for concurrent use. Type and field-name caches use `sync.Map` internally.

For maps with interface keys, plain strings use paths such as `labels[region]`.
A leading `@` in a string is doubled. Other keys include their Go type, for example
`labels[@int:1]`, and a nil interface key uses `labels[@nil]`. Ignore rules use the same paths.

Map key paths escape literal brackets and backslashes with a backslash. A key
`x][y` produces `labels[x\]\[y]`, distinct from the nested path `labels[x][y]`.
Pointer keys use pointer identity and their paths are specific to the process.
Unnamed interface key types use their full structural identity, including
the package paths of named constituent types.
When different Go types share a package and name, later types receive a numeric suffix such as `#2`. These disambiguated names are stable for the
life of the process. Positive and negative floating-point zero use the same path, including in complex keys.
