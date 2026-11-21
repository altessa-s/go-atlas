# query

```go
import "github.com/altessa-s/go-atlas/store/mongo/internal/query"
```

Internal query primitives shared by MongoDB pagination and repository reads.

## Responsibilities

| API | Purpose |
|-----|---------|
| `ParseSortString`, `ParseSortStringStrict` | Parse legacy comma-separated fields, optionally with an allowlist |
| `ParseSortOption` | Normalize string, pointer and BSON sort options |
| `Registry` | Shared default BSON encoding registry |
| `FilterEncoding` | Resolve read filter encoding from explicit settings, client settings, and Atlas defaults |
| `DefaultTimeout`, `CloseTimeout` | Read and cursor cleanup time limits |

The sort syntax is `name,-age`, not AIP `order_by`. Named string and pointer types are accepted by sort options. The [root package](../../README.md) uses this parser through its list sort options.

Read filters are prepared by `go-mongox/bsonfilter.Prepare` in the root package; see the [read filter contract](../../README.md#read-filters)
for supported documents and scalar codecs.
