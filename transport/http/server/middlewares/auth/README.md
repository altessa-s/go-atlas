# auth

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
```

Package `auth` provides HTTP authentication middleware with support for Bearer tokens, API keys, and custom token schemes. It offers flexible
token extraction, validation, and error handling with path-based filtering.

## Key types

| Type / Interface    | Description                                              |
|---------------------|----------------------------------------------------------|
| `middleware`        | Main authentication middleware implementation           |
| `AuthFunc`          | Interface for token validation                          |
| `TokenExtractor`    | Interface for extracting tokens from requests           |
| `ErrorHandler`      | Interface for custom error responses                    |

## Options

| Option                | Default                  | Description                         |
|-----------------------|--------------------------|-------------------------------------|
| `WithAuthFunc`        | --                       | Authentication function (required)  |
| `WithTokenExtractor`  | Bearer token extractor   | Custom token extraction logic       |
| `WithErrorHandler`    | Default 401 response     | Custom error response handler       |
| `WithIgnorePaths`     | --                       | Paths to skip authentication        |
| `WithIgnorePatterns`  | health, metrics patterns | Regex patterns for paths to skip    |
| `WithLogger`          | discard                  | Structured logger                   |

## Usage

### Bearer token authentication

```go
authFunc := auth.AuthenticateFunc(func(ctx context.Context, token string) (any, error) {
    // Validate token and return user data
    return validateToken(token)
})

middleware := auth.Middleware(
    auth.WithAuthFunc(authFunc),
)

handler := middleware(yourHandler)
```

### API key authentication

```go
extractor := auth.ExtractTokenFromHeader("X-API-Key", func(value string) (string, error) {
    return value, nil
})

middleware := auth.Middleware(
    auth.WithTokenExtractor(extractor),
    auth.WithAuthFunc(authFunc),
)
```

### Custom error handling

```go
errorHandler := auth.ErrorHandlerFunc(func(w http.ResponseWriter, r *http.Request, err error) {
    if err == auth.ErrMissingToken {
        w.WriteHeader(http.StatusUnauthorized)
        json.NewEncoder(w).Encode(map[string]string{"error": "token required"})
    }
})

middleware := auth.Middleware(
    auth.WithAuthFunc(authFunc),
    auth.WithErrorHandler(errorHandler),
)
```

### Retrieving auth data

```go
func handler(w http.ResponseWriter, r *http.Request) {
    authData := auth.FromContext(r.Context())
    if userInfo, ok := authData.(UserInfo); ok {
        fmt.Fprintf(w, "Hello, %s", userInfo.Name)
    }
}
```

## Subpackages

| Package              | Description                                                |
|----------------------|------------------------------------------------------------|
| [static](./static)   | Static token/API key validation with timing-safe comparison |

## Integration with middleware chain

```go
chain := middlewares.NewChain(
    realip.Middleware(),        // Extract client IP first
    requestid.Middleware(),     // Add request ID
    auth.Middleware(authOpts),  // Authenticate
    logger.Middleware(),        // Log authenticated requests
)

handler := chain.Then(yourHandler)
```

## Security considerations

- Always use HTTPS/TLS for token transmission
- Implement rate limiting to prevent brute force attacks
- Use timing-safe comparison for token validation (see static subpackage)
- Rotate tokens regularly
- Never log tokens in plaintext