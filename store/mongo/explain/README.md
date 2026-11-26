# explain

```go
import "github.com/altessa-s/go-atlas/store/mongo/explain"
```

Executes aggregation explain commands and logs query statistics. Used by the optional explain mode in [mongo](../README.md).

## API

| Symbol | Purpose |
|--------|---------|
| `Run` | Execute an aggregation explain command (`queryPlanner` by default); accepts `Option` values |
| `WithVerbosity` | Select `VerbosityQueryPlanner` (default) or `VerbosityExecutionStats` |
| `WithInsecureLogRawExplain` | Log the raw MongoDB explain document (including `parsedQuery` filter literals) at Debug |
| `Stats` | Index usage, examined and returned document counts, and execution time |
| `HighExaminationRatio` | Warning threshold for examined/returned ratio |

## Compatibility

`mongo.ExplainStats` remains an alias of `Stats`. Existing list options continue to work.
