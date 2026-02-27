# iso7064

```go
import "github.com/altessa-s/go-atlas/domain/validation/iso7064"
```

Package `iso7064` provides check-digit validation using the ISO 7064 MOD 11-10 algorithm for 9-digit numeric identifiers. Both
functions accept `string` or `int64` via a generic type parameter; pass strings when leading zeros must be preserved.

## Functions

| Function               | Description                                                                                            |
|------------------------|--------------------------------------------------------------------------------------------------------|
| `Mod11_10`             | Validate an identifier and return the boolean result together with an error on malformed input          |
| `IsValidMod11_10`      | Convenience wrapper that returns `false` on any error, suitable for simple guard checks                 |
