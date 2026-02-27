# bloom

```go
import "github.com/altessa-s/go-atlas/data/probfilter/bloom"
```

Package `bloom` provides a Bloom filter implementation for probabilistic existence checks. Bloom filters are space-efficient but do not support
deletion. Use periodic rebuilds via `RebuildableFilter` when the underlying dataset changes.
