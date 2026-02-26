# ocsp

```go
import "github.com/altessa-s/go-atlas/security/tlsutils/ocsp"
```

Package `ocsp` provides OCSP stapling for TLS certificates with automatic caching, gzip compression
(60-95% memory reduction), and scheduler-based refresh.

## Usage

```go
stapler := ocsp.NewOCSPStapler(
    ocsp.WithLogger(logger),
    ocsp.WithRetryPolicy(3, time.Second),
)

// Apply to TLS config
ocsp.StapleOCSPToConfig(tlsCfg, stapler)

// Scheduler-based refresh
sched.Register(ctx, scheduler.TaskConfig{
    ID:       "ocsp-refresh",
    Schedule: "@every 1h",
    Func:     stapler.RunRefreshCycle,
})
```

## Functions

| Function / Type       | Description                                 |
|-----------------------|---------------------------------------------|
| `NewOCSPStapler`      | Create stapler with caching and compression |
| `StapleOCSPToConfig`  | Apply stapler to `*tls.Config`              |
| `GetOCSPStaple`       | Get OCSP response for a certificate         |
| `RunRefreshCycle`      | Refresh cycle method for scheduler          |
| `RunRefreshAll`        | Refresh all cached OCSP responses           |
