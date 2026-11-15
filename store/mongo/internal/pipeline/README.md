# pipeline

```go
import "github.com/altessa-s/go-atlas/store/mongo/internal/pipeline"
```

Internal BSON aggregation builders for offset and cursor pagination. Callers handle validation, defaults and database execution.

## Builders

| Function | Responsibility |
|----------|----------------|
| `Offset` | Filter, sort, skip/limit, projection and optional facet count |
| `CursorItems` | Fetch one extra item, decorate and project |
| `CursorResult` | Shape items and count the original filter through an independent union |

Input documents are referenced without mutation. Bounds and effective sort must already be validated. Cursor predicates and custom item stages must not be passed as the count filter.

Used by the [root pagination APIs](../../README.md).
