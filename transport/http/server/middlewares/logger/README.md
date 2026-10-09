# logger

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/logger"
```

Package `logger` provides HTTP middleware for structured request/response logging. Logs method, path, status code, duration, and client
IP for each request. Supports configurable path filtering to exclude health-check and metrics endpoints from log output. Integrates with
the `realip` middleware for accurate client IP logging when behind proxies or load balancers.

## Options

| Option                    | Default                  | Description                                                  |
|---------------------------|--------------------------|--------------------------------------------------------------|
| `WithTimeFormat`          | `timeformat.RFC3339`     | Time format of logged timestamps                             |
| `WithLogRequest`          | off                      | Log the request body (redacted, see below)                   |
| `WithLogResponse`         | off                      | Log the response body (redacted, see below)                  |
| `WithBodyRedactor`        | full mask when bodies on | Scrubs logged bodies                                         |
| `WithLogQuery`            | off                      | Log the query string as `http.query`                         |
| `WithRedactQueryParams`   | --                       | Query parameters logged as `REDACTED`                        |
| `WithLogResponseCodes`    | `DefaultLogStatusCodes`  | Status codes that are logged                                 |
| `WithIgnoreResponseCodes` | --                       | Status codes that are not logged                             |
| `WithIgnoreMethods`       | --                       | HTTP methods to skip                                         |
| `WithIgnorePaths`         | --                       | Paths to skip                                                |
| `WithIgnorePatterns`      | health, metrics, pprof   | Regex patterns for paths to skip                             |
| `WithContextLogger`       | nil                      | Store the request-enriched logger in the context             |

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

## Query strings

The query string is not logged by default. `WithLogQuery()` (YAML `logQuery: true`) adds it as `http.query`; `WithRedactQueryParams(names...)`
(YAML `redactQueryParams`) logs the values of the listed parameters as `REDACTED`, keeping the order and encoding of the rest. Names match
exactly, after unescaping; both `&` and `;` count as separators.

## Levels

`Slog` logs 5xx, 401 and 403 at Error, other 4xx at Warn and everything else at Info. `SlogWithLevels(l, map[int]slog.Level{401: slog.LevelInfo})`
(YAML `statusLevels: {401: info}`) overrides the level per status code. `DefaultLogStatusCodes`, logged when `WithLogResponseCodes` is unset,
include the 303, 307 and 308 redirects.
