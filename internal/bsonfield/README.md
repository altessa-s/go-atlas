# bsonfield

```go
import "github.com/altessa-s/go-atlas/internal/bsonfield"
```

The store and translator use `Inspect` to classify a declared Go type and detect nested field selections, including through recursive and collection
types.

## Key types

| Type        | Purpose                                                              |
|-------------|----------------------------------------------------------------------|
| `Selection` | Distinguishes absent, dynamic, selected, and overlapping fields.     |
