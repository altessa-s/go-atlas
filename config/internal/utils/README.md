# utils

```go
import "github.com/altessa-s/go-atlas/config/internal/utils"
```

Internal package `utils` provides file-path helpers used by the config schemas' `Normalize` methods.

> **Note:** This is an internal package. API may change without notice.

## Functions

| Function      | Description                                                                 |
|---------------|-----------------------------------------------------------------------------|
| `ResolvePath` | Keep an absolute path, resolve a relative one against the working directory |

`ResolvePath` never checks existence and never drops a path: a missing file keeps its resolved path so the schema's `Validate` reports it instead of
the path silently becoming empty. An empty path stays empty.
