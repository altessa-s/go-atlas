# explain

```go
import "github.com/altessa-s/go-atlas/store/mongo/explain"
```

Executes aggregation explain commands and logs query statistics. Used by the optional explain mode in [mongo](../README.md).

## API

| Symbol | Purpose |
|--------|---------|
| `Run` | Execute an explain command with execution statistics; requires a non-nil logger |
| `Stats` | Index usage, examined and returned document counts, and execution time |
| `HighExaminationRatio` | Warning threshold for examined/returned ratio |

## Compatibility

`mongo.ExplainStats` remains an alias of `Stats`. Existing list options continue to work.
