# validator

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc/validator"
```

Package `validator` provides a production-ready OIDC token validator. `DefaultValidator` validates tokens via an OIDC `Provider` and extracts
standard claims (subject, email, scopes, issuer, audience, timestamps). Handles multiple scope formats: space-separated string, array, mixed array.

## Key types

| Type / Interface    | Description                                                                |
|---------------------|----------------------------------------------------------------------------|
| `DefaultValidator`  | Validates tokens and extracts structured `oidc.Claims` from raw claims     |
| `Provider`          | Minimal OIDC provider interface: `ValidateToken(ctx, token) (map, error)` |

## Options

| Option       | Default | Description                    |
|--------------|---------|--------------------------------|
| `WithLogger` | discard | Structured logger for errors   |
