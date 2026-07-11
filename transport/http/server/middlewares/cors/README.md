# cors

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/cors"
```

Package `cors` provides middleware for Cross-Origin Resource Sharing. Manages all standard CORS headers: Allow-Origin, Allow-Methods,
Allow-Headers, Allow-Credentials, Expose-Headers, and Max-Age. Supports preflight caching and private network access requests.

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
| `WithLogger`              | nil             | Structured logger for CORS events                                  |
| `WithIgnorePaths`         | --              | Paths to exclude from CORS processing                              |

## Credentials safety

`New` panics on constructor when `WithAllowCredentials` is combined with a wildcard origin policy, because the handler echoes the request `Origin`
(never a `*` wildcard) — so an over-broad policy plus credentials lets any site issue credentialed cross-origin requests and read the response. Two
combinations are rejected:

- `WithAllowAllOrigins` + `WithAllowCredentials`.
- `WithAllowCredentials` + a `WithAllowedOriginPatterns` regex that matches arbitrary origins (e.g. `.*`, `^https?://.*$`, or an empty pattern).

Domain-anchored patterns such as `^https://[a-z0-9-]+\.example\.com$` are unaffected. Anchor your patterns to your own domains.
