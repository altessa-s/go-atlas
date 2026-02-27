# cuckoo

```go
import "github.com/altessa-s/go-atlas/data/probfilter/cuckoo"
```

Package `cuckoo` provides a Cuckoo filter implementation for probabilistic existence checks. Unlike Bloom filters, Cuckoo filters support
deletion of individual items at the cost of slightly higher memory overhead.
