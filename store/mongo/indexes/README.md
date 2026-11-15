# indexes

```go
import "github.com/altessa-s/go-atlas/store/mongo/indexes"
```

Creates and reconciles collection indexes using the MongoDB driver. Import this package directly; the root no longer re-exports index reconciliation.

## API

| Symbol | Purpose |
|--------|---------|
| `Ensure` | Create the declared indexes; equal specifications are a no-op |
| `Option` | Reconciliation option |
| `ErrInvalidIndexModels` | Invalid keys, duplicate names, or invalid obsolete-index declarations |

## Options

| Option | Default | Effect |
|--------|---------|--------|
| `WithDropObsolete` | None | Drop the explicitly named indexes first |
| `WithDropOnSpecMismatch` | Disabled | Reconcile conflicts by name under a per-collection lease |

Dropping a unique index removes its constraint until the replacement finishes. A conflict with an index of another name is returned without dropping that index. Migration and reconciliation leases share an internal implementation and collection, using different document IDs.
