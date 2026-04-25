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
