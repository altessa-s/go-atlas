# noop

```go
import "github.com/altessa-s/go-atlas/data/locks/dlock/providers/noop"
```

Package `noop` implements a no-op distributed lock provider. Until `Close`, every lock acquisition succeeds immediately without coordination; after it,
`Lock` fails. Useful for testing and single-instance deployments where distributed locking is not required.
