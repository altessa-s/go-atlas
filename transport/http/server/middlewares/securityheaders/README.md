# securityheaders

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/securityheaders"
```

Package `securityheaders` provides middleware for adding security-related HTTP response headers. Protects against common web
vulnerabilities including XSS, clickjacking, and MIME sniffing attacks.

## Default headers

| Header                   | Default value                          | Description                            |
|--------------------------|----------------------------------------|----------------------------------------|
| `X-Content-Type-Options` | `nosniff`                              | Prevents MIME type sniffing            |
| `X-Frame-Options`        | `DENY`                                 | Prevents clickjacking via iframes      |
| `Referrer-Policy`        | `strict-origin-when-cross-origin`      | Controls referrer information leakage  |
| `X-XSS-Protection`      | `0`                                    | Disabled (modern browsers have built-in) |

## Optional headers

| Header                     | Option                          | Description                              |
|----------------------------|---------------------------------|------------------------------------------|
| `Strict-Transport-Security`| `WithHstsEnabled`, `WithHstsMaxAge`  | Enforces HTTPS connections (HSTS)   |
| `Content-Security-Policy`  | `WithContentSecurityPolicy`     | Controls resource loading policies       |
| `Permissions-Policy`       | `WithPermissionsPolicy`         | Controls browser feature access          |
| `Cross-Origin-Opener-Policy`  | `WithCrossOriginOpenerPolicy`   | Browsing-context isolation (COOP)     |
| `Cross-Origin-Embedder-Policy`| `WithCrossOriginEmbedderPolicy` | Cross-origin isolation (COEP) — disruptive |
| `Cross-Origin-Resource-Policy`| `WithCrossOriginResourcePolicy` | Who may embed this response (CORP)    |

All optional headers are off by default — the middleware already ships secure-by-default for the non-breaking headers, while CSP, HSTS and the Cross-Origin-* trio stay opt-in because a blanket default would break real applications.

## Presets

`RecommendedOptions()` and `StrictOptions()` bundle a safe baseline as opt-in `[]Option` you splat into `New`:

```go
mw := securityheaders.New(securityheaders.RecommendedOptions()...)
```

- `RecommendedOptions()` — pins the non-breaking defaults (`X-Frame-Options: DENY`, strict `Referrer-Policy`, `nosniff`, `X-XSS-Protection: 0`) and adds
  a conservative `Permissions-Policy` (`RecommendedPermissionsPolicy`) that denies powerful features. It never sets CSP, HSTS or any Cross-Origin-*
  header.
- `StrictOptions()` — `RecommendedOptions()` plus `Cross-Origin-Opener-Policy: same-origin` and `Cross-Origin-Resource-Policy: same-origin`. It still
  omits `Cross-Origin-Embedder-Policy` (require-corp is the most disruptive header — enable it explicitly with `WithCrossOriginEmbedderPolicy`) and
  CSP/HSTS.

CSP and HSTS remain the service's responsibility: they are application- and deployment-specific and can break a working site if imposed blindly.
Append your own options after a preset to override any entry.
