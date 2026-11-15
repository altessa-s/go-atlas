# defaults

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/defaults"
```

Package `defaults` provides shared default configurations and constants for HTTP middlewares. Centralizes common default values such as ignore patterns
for health-check and metrics endpoints, so middleware implementations stay consistent instead of each defining its own copy.

The patterns are anchored: they match the health, readiness, liveness, metrics, and pprof endpoints at the root or directly under `/internal` (the
server factory's default prefix), never an application route that merely ends in the same segment (`/users/metrics`, `/x/pprof/heap`). For built-in
endpoints mounted under another prefix, use `IgnorePatternsWithPrefix(prefix)`.

Observability middleware (logger, tracing, metrics, ...) skips these paths by default. Access-control and safety middleware ([`auth`](../auth),
[`ipacl`](../ipacl), [`geoacl`](../geoacl), [`limiter`](../limiter), [`recovery`](../recovery)) exempts nothing unless configured; pass
`WithIgnorePatterns(defaults.IgnorePatterns...)` to opt in explicitly.
