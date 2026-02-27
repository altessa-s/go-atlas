# logger

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/logger"
```

Package `logger` provides HTTP middleware for structured request/response logging. Logs method, path, status code, duration, and client
IP for each request. Supports configurable path filtering to exclude health-check and metrics endpoints from log output. Integrates with
the `realip` middleware for accurate client IP logging when behind proxies or load balancers.
