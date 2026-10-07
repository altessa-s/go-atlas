# validationconfig

```go
import validationconfig "github.com/altessa-s/go-atlas/config/validation"
```

Package `validationconfig` defines shared validation helpers for configuration schemas: struct validation with readable errors and storage-selector
checks. Schemas are populated by [`config/loader`](../loader) and consumed by the component factories, which map them to generated options; runtime
packages never import them.

## Key types

| Type          | Description                                                                                                        |
|---------------|--------------------------------------------------------------------------------------------------------------------|
| `StorageCase` | Pairs a storage type value with the pointer to the sub-config that must be set when the type selector equals When. |

See the [config index](../README.md) for the other schema packages.
