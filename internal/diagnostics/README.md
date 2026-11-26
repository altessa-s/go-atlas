# diagnostics

```go
import "github.com/altessa-s/go-atlas/internal/diagnostics"
```

Shared error formatting for logs, span events and persisted diagnostics.

## Key functions

| Function | Purpose |
|----------|---------|
| `LogError` | Formats bounded text and strips URL user information, scheme-less DSN credentials, and `password=`/`pwd=` values when a record is emitted. |
| `ErrorClass` | Classifies an external failure without rendering its text. |
| `ClassifiedError` | Exposes a trusted classification and preserves error identity for `errors.Is`. |
