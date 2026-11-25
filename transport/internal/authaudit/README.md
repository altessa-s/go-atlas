# authaudit

```go
import "github.com/altessa-s/go-atlas/transport/internal/authaudit"
```

HTTP and gRPC authentication adapters use `Record` to emit the same decision fields. Each adapter maps recorder failures to its transport status.
