# ocsp

```go
import "github.com/altessa-s/go-atlas/security/tlsutils/ocsp"
```

Package `ocsp` provides OCSP stapling for TLS certificates with automatic caching, gzip compression (60-95% memory reduction), and
scheduler-based refresh.

## Functions

| Function / Type       | Description                                 |
|-----------------------|---------------------------------------------|
| `NewOCSPStapler`      | Create stapler with caching and compression |
| `StapleOCSPToConfig`  | Apply stapler to `*tls.Config`              |
| `GetOCSPStaple`       | Get OCSP response for a certificate         |
| `RunRefreshCycle`      | Refresh cycle method for scheduler          |
| `RunRefreshAll`        | Refresh all cached OCSP responses           |
