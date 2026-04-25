# audit

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/audit"
```

Package `audit` provides HTTP middleware for automatic request auditing. Delegates to a `data/audit.Auditor` for non-blocking event
emission. The middleware captures request method, path, status code, duration, and client identity, then emits an audit event after the
handler completes. Declares dependencies on `requestid`, `realip`, and `tracing` middlewares for context enrichment.
