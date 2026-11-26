# migrations

```go
import "github.com/altessa-s/go-atlas/store/mongo/migrations"
```

Registers and runs versioned schema migrations. Uses the existing mongo-migrate record format and a separate database lease collection.

## API

| Symbol | Purpose |
|--------|---------|
| `RegisterMigration` | Register an up/down step |
| `Registry.Register` | Add a step to an independent, zero-value-ready registry |
| `WithRegistry` | Apply only the supplied registry; omit to merge the default and legacy registrations |
| `RegisteredMigrations` | Sorted registry snapshot |
| `Run` | Apply pending migrations to a driver database |
| `WithLogger` | Log rollback and lock release failures |
| `WithRollbackOnFailure` | Opt into rollback of the steps applied by this run on failure |

The [root package](../README.md) runs these migrations during Connect. Register steps through this package directly. Registration should finish before running migrations. The lease uses server time and is renewed every third of a lease until the work returns, so a step that ignores context cancellation does not overlap a successor while renewals succeed; a failed renewal cancels the work and skips rollback. A step registered with a nil down is irreversible (`ErrIrreversibleMigration`).
