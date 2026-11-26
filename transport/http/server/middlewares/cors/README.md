# cors

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/cors"
```

Package `cors` provides middleware for Cross-Origin Resource Sharing. Manages all standard CORS headers: Allow-Origin, Allow-Methods, Allow-Headers,
Allow-Credentials, Expose-Headers, and Max-Age. Supports preflight caching and private network access requests.

## Options

| Option                    | Default         | Description                                                        |
|---------------------------|-----------------|--------------------------------------------------------------------|
| `WithAllowedOrigins`      | --              | Origins allowed to make cross-origin requests                      |
| `WithAllowAllOrigins`     | false           | Allow requests from any origin (use with caution)                  |
| `WithAllowedOriginPatterns` | --            | Regex patterns for dynamically matching allowed origins            |
| `WithAllowedMethods`      | standard set    | HTTP methods allowed for cross-origin requests                     |
| `WithAllowedHeaders`      | standard set    | Headers that can be used in cross-origin requests                  |
| `WithExposedHeaders`      | none            | Headers that browsers are allowed to access                        |
| `WithAllowCredentials`    | false           | Allow credentials (cookies, auth headers, TLS client certs)        |
| `WithMaxAge`              | 86400           | Preflight result cache duration in seconds                         |
| `WithAllowPrivateNetwork` | false           | Enable Private Network Access preflight support                    |
| `WithOptionsPassthrough`  | false           | Pass OPTIONS requests to next handler instead of handling them     |
| `WithOptionsSuccessStatus`| 204             | HTTP status code for successful OPTIONS responses                  |
| `WithLogger`              | discard         | Structured logger for CORS events                                  |
| `WithIgnorePaths`         | --              | Paths to exclude from CORS processing                              |
| `WithIgnorePatterns`      | health, metrics, pprof | Regex patterns for paths to exclude from CORS processing    |

## Credentials safety

Credentialed responses echo the request `Origin`. `WithAllowedOrigins` and YAML `allowedOrigins` compare each entry literally. Use
`WithAllowedOriginPatterns` or YAML `allowedOriginPatterns` for regular expressions. With credentials, start and end anchors must apply to every
alternative, and `New` rejects `WithAllowAllOrigins` and patterns that match arbitrary origins.

Start each host wildcard, including a character class or variable-width repetition, immediately after a literal dot (`\.`) or `://`. Every
alternative with a host wildcard must keep the registrable domain and its entire public suffix literal, with a literal dot separating them from
any variable host part. The port may use decimal digit classes and repeats.

`^https://[a-z0-9-]+\.example\.com$` and `^https://app\..*\.example\.com:8[0-9]{3}$` are accepted. `^https://app[^.]\.example\.com$` and
`^https://example\.com\..*$` are rejected.

## Caching

Every response that passes through the middleware (not ignored) carries `Vary: Origin`, whether the request had an allowed, a disallowed, or no
`Origin` header, so a shared cache never serves one origin's response to another. Preflight responses also vary on
`Access-Control-Request-Method` and `Access-Control-Request-Headers`.
