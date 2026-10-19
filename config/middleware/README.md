# middlewareconfig

```go
import middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
```

Package `middlewareconfig` defines schema pieces shared by gRPC interceptors and HTTP middlewares: enable toggles, fallback behavior and IP/geo ACL
rules. Schemas are populated by [`config/loader`](../loader) and consumed by the component factories, which map them to generated options; runtime
packages never import them.

## Key types

| Type               | Description                                                                                        |
|--------------------|----------------------------------------------------------------------------------------------------|
| `GeoACLRule` | Defines a geographic access control rule for one or more endpoints.                                |
| `IPACLRule`  | Defines an IP access control rule for one or more endpoints.                                       |
| `EnableMixin`      | Provides a common Enabled field for configurations that can be toggled.                            |
| `FallbackBehavior` | Defines how interceptors behave when encountering errors or when storage backends are unavailable. |
| `IdempotencyKeyFormat` | Validator idempotency keys must pass: `uuidv4` (default), `opaque` or `pattern`; `IdempotencyKeyFormatRule` validates it. |

`IdempotencyKeyRules` returns the field rules the gRPC and HTTP idempotency configs share: key format, key pattern, maximum key length, and the
mandatory list and patterns.

See the [config index](../README.md) for the other schema packages.
