# ptr

```go
import "github.com/altessa-s/go-atlas/core/types/ptr"
```

Package `ptr` provides helpers for creating and dereferencing pointers to primitive types. Useful for struct initialization with optional fields or 
APIs requiring pointer values. All functions are pure and thread-safe.

## Functions

| Function      | Description                                             |
|---------------|---------------------------------------------------------|
| `Wrap`        | `T` -> `*T` (create pointer to a literal or expression) |
| `WrapNonZero` | `T` -> `*T`, returns `nil` for zero values              |
| `Unwrap`      | `*T` -> `T`, with optional default when `nil`           |

## Usage

```go
cfg := Config{
    Timeout: ptr.Wrap(30),
    Name:    ptr.WrapNonZero(name), // nil if name is empty
}

value := ptr.Unwrap(cfg.Timeout)       // 30
name  := ptr.Unwrap(cfg.Name, "default") // "default" if nil
```
