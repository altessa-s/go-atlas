# ocsp

```go
import "github.com/altessa-s/go-atlas/security/tlsutils/ocsp"
```

Package `ocsp` provides OCSP stapling for TLS certificates with automatic caching and scheduler-based refresh. A fetched response is stapled only when
it is signed by the issuer, names the leaf's serial number, reports `good`, and is within its validity window (`thisUpdate`/`nextUpdate`, 5 minutes of
clock skew tolerated); otherwise `GetOCSPStaple` fails (`ErrStaleResponse` for a stale response) and the stapler's `FailureMode` decides the handshake
outcome. A response without a `nextUpdate` is accepted only until its `thisUpdate` plus `WithMaxResponseAge` (default `DefaultMaxResponseAge`, 24h) and
is never cached past that deadline. `WithCompression` requests gzip-encoded responder replies; the cache always holds the raw DER response.

## Functions

| Function / Type       | Description                                 |
|-----------------------|---------------------------------------------|
| `NewOCSPStapler`      | Create stapler with caching                 |
| `StapleOCSPToConfig`  | Apply stapler to `*tls.Config`              |
| `StapleCertificate`   | Staple one certificate honoring `FailureMode`, for per-certificate callbacks |
| `GetOCSPStaple`       | Get OCSP response for a certificate         |
| `RunRefreshCycle`      | Refresh cycle method for scheduler          |
| `RunRefreshAll`        | Refresh all cached OCSP responses           |
| `RegisterRefreshAllSchedulerFunc` | Scheduler task func for refresh-all; direct `RunRefreshAll` calls then return `ErrSchedulerManaged` |
| `NeedsRefresh`        | Report whether a certificate's cached response needs refreshing |

## StapleOCSPToConfig and SNI

crypto/tls serves a certificate from `Config.Certificates` without calling `GetCertificate` when the client sends no SNI, which would skip stapling and
Hard-mode enforcement. `StapleOCSPToConfig` therefore moves `Certificates` (and `NameToCertificate`) into the wrapped `GetCertificate`, leaving those
fields empty, and selects among them exactly as crypto/tls does. When `GetClientCertificate` is unset, it installs one that selects from the moved
certificates like crypto/tls and, as before, does not staple. Call it after `Certificates` is populated and do not repopulate `Certificates` afterwards.
Configs returned by `GetConfigForClient` are not wrapped — apply `StapleOCSPToConfig` to each. Enforcement applies to full handshakes; a resumed session
does not select a certificate again.
