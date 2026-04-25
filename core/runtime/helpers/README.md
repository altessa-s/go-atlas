# helpers

```go
import "github.com/altessa-s/go-atlas/core/runtime/helpers"
```

Package `helpers` provides low-level runtime utilities for debugging and tracing. Uses `runtime.Stack` internally — not for hot paths.

## Functions

| Function      | Description                                           |
|---------------|-------------------------------------------------------|
| `GoroutineID` | Return the numeric ID of the calling goroutine        |

The ID is extracted by parsing `runtime.Stack` output. This allocates a small buffer on each call — use sparingly in hot paths.
