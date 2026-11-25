# numericdate

```go
import "github.com/altessa-s/go-atlas/auth/internal/numericdate"
```

JWT verification and OIDC policy use this package to decode registered dates and compare them to a clock or duration with exact decimal precision.

## Key types

| Type | Purpose |
|------|---------|
| Date | A Unix timestamp in the signed 64-bit seconds range with an optional fractional part. |
