# validators

```go
import "github.com/altessa-s/go-atlas/config/internal/validators"
```

Internal package `validators` provides custom [ozzo-validation](https://github.com/go-ozzo/ozzo-validation) rules for configuration fields.

> **Note:** This is an internal package. API may change without notice.

## Rules

### Network addresses

| Rule                                     | Accepted values                                                                                     |
|------------------------------------------|-----------------------------------------------------------------------------------------------------|
| `CIDRPrefix`                             | IPv4 or IPv6 CIDR prefix with an explicit prefix length                                             |
| `HTTPSURL(allowHTTP, allowLoopbackHTTP)` | Empty string or absolute HTTPS URL. HTTP can be enabled for all hosts or only for loopback hosts. |

### MongoDirectionConnect

Validates that MongoDB `DirectConnect` is not enabled when using multiple hosts or SRV connection strings.

The rule rejects `DirectConnect=true` when:

- Multiple hosts are specified
- Any host uses the `mongodb+srv://` scheme

### RetryQueueFlagField

Checks a MongoDB retry queue flag against the storage-owned names and its companion flag.

### SQL names

| Rule            | Accepted names                                                                        |
|-----------------|---------------------------------------------------------------------------------------|
| `SQLTableName`  | Plain identifier, optionally qualified by one schema; each part at most 63 characters |
| `SQLIdentifier` | Plain identifier without a qualifier, at most 63 characters                           |

Both rules use [sqlident](../../../internal/sqlident) to validate names. They accept `[A-Za-z_][A-Za-z0-9_]*` and let empty strings pass; pair with
`validation.Required` when needed.
`BackendTypes[T]` adds the names supplied by the application to a schema’s built-in selector values. `SetRegisteredTypes` snapshots its input.
