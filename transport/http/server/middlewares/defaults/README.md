# defaults

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/defaults"
```

Package `defaults` provides shared default configurations and constants for HTTP middlewares. Centralizes common default values such as
ignore patterns for health-check and metrics endpoints, so middleware implementations stay consistent instead of each defining its own copy.

The authentication middleware ([`auth`](../auth)) does not apply `IgnorePatterns` by default: the patterns are unanchored at the start
(`/admin/metrics` and `/api/pprof/heap` match), so authentication exempts nothing unless configured. Pass
`auth.WithIgnorePatterns(defaults.IgnorePatterns...)` to opt in explicitly.
