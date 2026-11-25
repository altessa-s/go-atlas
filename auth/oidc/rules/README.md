# rules

```go
import "github.com/altessa-s/go-atlas/auth/oidc/rules"
```

The OIDC provider and the [service configuration parser](../../../config/auth) share the CEL environment and validate leeway and token lifetime bounds.

## Validation

| Function              | Contract                                                           |
|-----------------------|--------------------------------------------------------------------|
| `ValidateCEL`         | Check an expression without populating the preset matcher cache.   |
| `CompileCEL`          | Prepare a bounded-cost boolean claims matcher for a policy.        |
| `CachedCEL`           | Reuse a compiled matcher for programmatic and JSON preset rules.   |
| `CheckLeeway`         | Reject a clock tolerance above the JWT verifier maximum.           |
| `CheckTokenLifetime`  | Reject a negative minimum or a minimum above an enabled maximum.   |
