# validators

```go
import "github.com/altessa-s/go-atlas/config/internal/validators"
```

Internal package `validators` provides custom [ozzo-validation](https://github.com/go-ozzo/ozzo-validation) rules
for configuration fields.

> **Note:** This is an internal package. API may change without notice.

## Rules

### MongoDirectionConnect

Validates that MongoDB `DirectConnect` is not enabled when using multiple hosts or SRV connection strings.

The rule rejects `DirectConnect=true` when:
- Multiple hosts are specified
- Any host uses the `mongodb+srv://` scheme
