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
| `WithMaxDepth`     | Maximum recursion depth (default `10`, `0` = off)  |
| `WithTagName`      | Struct tag for field names (default `"json"`)       |

## Concurrency

`Tracker` is safe for concurrent use. Type and field-name caches use `sync.Map` internally.
