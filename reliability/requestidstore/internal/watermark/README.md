# watermark

```go
import "github.com/altessa-s/go-atlas/reliability/requestidstore/internal/watermark"
```

The SQL and NATS purge loops use the latest observed caller time as their shared cutoff for each pass.

## Key types

| Type    | Purpose                                    |
|---------|--------------------------------------------|
| `Clock` | Concurrent updates and reads of the cutoff. |
