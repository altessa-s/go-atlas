# probfiltertest

```go
import "github.com/altessa-s/go-atlas/cache/probfilter/probfiltertest"
```

Test fixture for consumers of [`factory/cache/probfilter`](../../../factory/cache/probfilter): an in-memory Bloom storage shared between processes,
like
the Redis one, so shared-filter rebuild behavior can be tested without Redis.

| Function                         | Description                                                                                                                   |
|----------------------------------|-------------------------------------------------------------------------------------------------------------------------------|
| `NewSharedBloom(peerRebuilding)` | Shared Bloom storage to register as the redis type; with `peerRebuilding` the rebuild lease fails with `ErrRebuildInProgress` |
