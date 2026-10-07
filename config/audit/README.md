# auditconfig

```go
import auditconfig "github.com/altessa-s/go-atlas/config/audit"
```

Package `auditconfig` defines the audit event dispatcher schema. Schemas are populated by [`config/loader`](../loader) and consumed by the component
factories, which map them to generated options; runtime packages never import them.

## Key types

| Type                | Description                                                   |
|---------------------|---------------------------------------------------------------|
| `StorageType`  | Defines the type of storage backend for audit events.         |
| `Config`             | Defines the configuration for the audit subsystem.            |
| `Storage`      | Defines the storage backend configuration for audit events.   |
| `StorageMongo` | Holds MongoDB-specific configuration for audit event storage. |

See the [config index](../README.md) for the other schema packages.
