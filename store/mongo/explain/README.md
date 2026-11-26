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
| `WithInsecureLogRawExplain` | Log the MongoDB explain result at Debug with the contents of `$literal` expressions redacted |
| `Stats` | Index usage, examined and returned document counts, and execution time |
| `HighExaminationRatio` | Warning threshold for examined/returned ratio |

An unknown verbosity causes `Run` to return an invalid argument error during command construction.

## Compatibility

`mongo.ExplainStats` remains an alias of `Stats`. Existing list options continue to work.
