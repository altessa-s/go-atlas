# noop

```go
import "github.com/altessa-s/go-atlas/data/uniq/providers/noop"
```

Package `noop` implements a no-op unique value provider. Every existence check returns false. Useful for testing and as a default when uniqueness
tracking is not required.
