# utils

```go
import "github.com/altessa-s/go-atlas/config/internal/utils"
```

Internal package `utils` provides file-lookup helpers used by the config loader and validators.

> **Note:** This is an internal package. API may change without notice.

## Functions

| Function   | Description                                               |
|------------|-----------------------------------------------------------|
| `FindFile` | Return the path if the file exists, empty string otherwise|

`FindFile` only validates absolute paths. Relative paths always return an empty string.
