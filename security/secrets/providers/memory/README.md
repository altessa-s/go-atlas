# memory

```go
import "github.com/altessa-s/go-atlas/security/secrets/providers/memory"
```

Package `memory` provides an in-memory secret storage provider. Implements the `Static` interface, so `Manager` skips periodic refresh.
Useful for testing or fixed secret sets.
