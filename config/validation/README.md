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

## Functions

| Function                  | Description                                                                                                     |
|---------------------------|-----------------------------------------------------------------------------------------------------------------|
| `ValidateStruct`          | `validation.ValidateStruct` with actionable messages for a field rule that does not point into the struct.      |
| `ValidateStructIfEnabled` | `ValidateStruct` gated on an `Enabled` flag.                                                                    |
| `NestedField`             | Field rule for a struct held by value whose `Validate` has a pointer receiver — ozzo skips it otherwise.         |
| `ValidateStorage`         | Checks a storage selector and that the sub-config of the selected type is set.                                 |
| `PrettyError`             | Renders a validation error as readable text.                                                                    |

Hold nested schemas by value and validate them with `NestedField(&c.Sub)`: `validation.Field(&c.Sub)` passes ozzo a copy of the value,
which lacks the pointer method set, so `Sub.Validate` is silently never called.

See the [config index](../README.md) for the other schema packages.
