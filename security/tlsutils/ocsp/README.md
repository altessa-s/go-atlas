# ocsp

```go
import "github.com/altessa-s/go-atlas/security/tlsutils/ocsp"
```

Package `ocsp` provides OCSP stapling for TLS certificates with automatic caching and scheduler-based refresh. A fetched response
is stapled only when it is signed by the issuer, names the leaf's serial number, reports `good`, and is within its validity window
(`thisUpdate`/`nextUpdate`, 5 minutes of clock skew tolerated); otherwise `GetOCSPStaple` fails (`ErrStaleResponse` for a stale
response) and the stapler's `FailureMode` decides the handshake outcome. `WithCompression` requests gzip-encoded responder replies; the
cache always holds the raw DER response.

## Functions

| Function / Type       | Description                                 |
|-----------------------|---------------------------------------------|
| `NewOCSPStapler`      | Create stapler with caching                 |
| `StapleOCSPToConfig`  | Apply stapler to `*tls.Config`              |
| `GetOCSPStaple`       | Get OCSP response for a certificate         |
| `RunRefreshCycle`      | Refresh cycle method for scheduler          |
| `RunRefreshAll`        | Refresh all cached OCSP responses           |
