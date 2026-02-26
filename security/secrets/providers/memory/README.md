# memory

```go
import "github.com/altessa-s/go-atlas/security/secrets/providers/memory"
```

Package `memory` provides an in-memory secret storage provider. Implements the `Static` interface, allowing
`Manager` to skip periodic refresh for optimal performance. Useful for testing or fixed secret sets.

## Usage

```go
store := memory.New(map[string]*secrets.Value[MySecret]{
    "api-key": secrets.NewValue[MySecret](...),
})

mgr := secrets.New[MySecret](store)
```
