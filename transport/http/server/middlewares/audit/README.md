# audit

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/audit"
```

Package `audit` provides HTTP middleware for automatic request auditing. Accepts an `EventEmitter` for non-blocking event emission; `audit.Auditor`
implements the interface. The middleware captures request method, path, status code, duration, and client identity, then emits an audit event after
the handler completes. Declares dependencies on `requestid`, `realip`, and `tracing` middlewares for context enrichment.

## Key types

| Type | Description |
|---|---|
| `EventEmitter` | Emits an `*audit.Event`; the boolean result is best effort |

## Stored lengths

The URL path and the User-Agent are client-controlled, so the middleware caps what one request can write into the audit store. A longer value is
truncated, never rejected; the cut is marked with `TruncationSuffix` (`...`), which counts toward the cap. The cap applies to the User-Agent of the
actor extractor as well. A cap below 1 is ignored.

| Option                   | Default                                 | Description                                |
|--------------------------|-----------------------------------------|--------------------------------------------|
| `WithMaxPathLength`      | `DefaultMaxPathLength` (2048 bytes)     | Cap of the stored `Resource.Path`          |
| `WithMaxUserAgentLength` | `DefaultMaxUserAgentLength` (512 bytes) | Cap of the stored `Actor.UserAgent`        |
