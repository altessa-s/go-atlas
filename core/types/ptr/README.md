# ptr

```go
import "github.com/altessa-s/go-atlas/core/types/ptr"
```

Package `ptr` provides helpers for creating and dereferencing pointers to primitive types. Useful for struct initialization with optional
fields or APIs requiring pointer values. All functions are pure, generic, and safe for concurrent use.

## Functions

| Function      | Description                                             |
|---------------|---------------------------------------------------------|
| `Wrap`        | `T` -> `*T` (create pointer to a literal or expression) |
| `WrapNonZero` | `T` -> `*T`, returns `nil` for zero values              |
| `Unwrap`      | `*T` -> `T`, with optional default when `nil`           |
