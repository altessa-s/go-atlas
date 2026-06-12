# defaults

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/defaults"
```

Package `defaults` provides shared default configurations and constants for HTTP middlewares. Centralizes common default values such as
ignore patterns for health-check and metrics endpoints, so middleware implementations stay consistent instead of each defining its own copy.
