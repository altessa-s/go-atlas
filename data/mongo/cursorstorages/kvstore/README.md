# kvstore

```go
import "github.com/altessa-s/go-atlas/data/mongo/cursorstorages/kvstore"
```

Package `kvstore` implements cursor storage using a generic key-value store interface for MongoDB pagination state persistence.

`NewJSONStorage(backend, name)` wraps a backend that implements the package's `Storage` interface and adds JSON serialization of cursor metadata, so the
result implements `mongo.CursorStorage`. A backend returns `ErrKeyNotFound` for a missing or expired key.
