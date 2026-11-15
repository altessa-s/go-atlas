# query

```go
import "github.com/altessa-s/go-atlas/store/mongo/internal/query"
```

Internal query primitives shared by MongoDB pagination and repository reads.

## Responsibilities

| API | Purpose |
|-----|---------|
| `ValidateFilter` | Reject server-side JavaScript operators in filters |
| `ParseSortString`, `ParseSortStringStrict` | Parse legacy comma-separated fields, optionally with an allowlist |
| `ParseSortOption` | Normalize string, pointer and BSON sort options |
| `Registry` | Shared default BSON encoding registry |
| `DefaultTimeout`, `CloseTimeout` | Read and cursor cleanup time limits |

The sort syntax is `name,-age`, not AIP `order_by`. Named string and pointer types are accepted by sort options. The [root package](../../README.md) uses this parser through its list sort options.
