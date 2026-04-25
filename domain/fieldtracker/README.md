# fieldtracker

```go
import "github.com/altessa-s/go-atlas/domain/fieldtracker"
```

Package `fieldtracker` detects changed fields between two struct instances. Field paths are returned in
dot notation (e.g. `"address.city"`) using names derived from struct tags or PascalCase-to-snake_case
conversion. Nested structs, embedded structs, slices, and maps are traversed recursively.

## Options

| Option             | Description                                        |
|--------------------|----------------------------------------------------|
| `WithIgnoreFields` | Field paths to skip (dot notation)                 |
| `WithMaxDepth`     | Maximum recursion depth (default `10`, `0` = off)  |
| `WithTagName`      | Struct tag for field names (default `"json"`)       |

## Concurrency

`Tracker` is safe for concurrent use. Type and field-name caches use `sync.Map` internally.
