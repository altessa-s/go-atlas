# constraints

```go
import "github.com/altessa-s/go-atlas/core/types/constraints"
```

Package `constraints` provides reusable generic type constraints for numbers, primitives, and strings. Covers all Go built-in numeric types.

## Constraints

| Constraint      | Permits                                                        |
|-----------------|----------------------------------------------------------------|
| `Signed`        | `~int`, `~int8`, `~int16`, `~int32`, `~int64`                  |
| `Unsigned`      | `~uint`, `~uint8`, `~uint16`, `~uint32`, `~uint64`, `~uintptr` |
| `Integer`       | `Signed` \| `Unsigned`                                         |
| `Float`         | `~float32`, `~float64`                                         |
| `Numbers`       | `Float` \| `Integer`                                           |
| `NumbersString` | `Numbers` \| `~string`                                         |
| `Primitive`     | `Numbers` \| `~string` \| `~bool`                              |
