# kvstore

```go
import "github.com/altessa-s/go-atlas/store/mongo/storages/internal/kvstore"
```

Package `kvstore` implements cursor storage using a generic key-value store interface for MongoDB pagination state persistence.

`NewJSONStorage(backend, name)` wraps a backend that implements the package's `Storage` interface and adds JSON serialization of cursor metadata, so
the
result implements `mongo.Storage`. A backend returns `ErrKeyNotFound` for a missing or expired key.
