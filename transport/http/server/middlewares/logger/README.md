# logger

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/logger"
```

Package `logger` provides HTTP middleware for structured request/response logging. Logs method, path, status code, duration, and client
IP for each request. Supports configurable path filtering to exclude health-check and metrics endpoints from log output. Integrates with
the `realip` middleware for accurate client IP logging when behind proxies or load balancers.

## Body logging is redacted by default

`WithLogRequest` / `WithLogResponse` capture request and response bodies, which routinely contain credentials, tokens, or PII. To keep those out
of logs, enabling body logging **without** a `WithBodyRedactor` installs a fully-masking redactor: every body is replaced with
`DefaultRedactedBodyPlaceholder`. Provide a real redactor to log scrubbed content, or an identity redactor to opt into raw bodies:

```go
// Scrub sensitive fields:
logger.New(h, logger.WithLogRequest(), logger.WithBodyRedactor(myScrubber))

// Opt into raw bodies (explicit, not the default):
logger.New(h, logger.WithLogRequest(), logger.WithBodyRedactor(func(b string) string { return b }))
```
