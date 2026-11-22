# bsonfield

```go
import "github.com/altessa-s/go-atlas/internal/bsonfield"
```

The store supplies a field transformer for one conversion. The translator selects BSON names, resolves Go field paths, and supplies the encoder for
the original field value. A failed transformation aborts the document.

`Inspect` classifies a declared type and detects a selected field inside another selected field, including through recursive and collection types.

## Key types

| Type | Purpose |
|------|---------|
| `Transformer` | Select fields by declaring type and replace their encoded values. |
