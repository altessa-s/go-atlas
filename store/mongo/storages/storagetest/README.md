# storagetest

```go
import "github.com/altessa-s/go-atlas/store/mongo/storages/storagetest"
```

Conformance checks for cursor storage implementations.

## Usage

Pass the backend's expected result for a stored nil. Memory preserves nil. The JSON-backed Redis and NATS stores return zero-valued metadata.

```go
storagetest.MetadataOwnership(t, memoryStore, nil)
storagetest.MetadataOwnership(t, redisStore, &mongo.CursorMetadata{})
```
