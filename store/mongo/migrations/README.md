# migrations

```go
import "github.com/altessa-s/go-atlas/store/mongo/migrations"
```

Registers and runs versioned schema migrations. Uses the existing mongo-migrate record format and a separate database lease collection.

## API

| Symbol | Purpose |
|--------|---------|
| `RegisterMigration` | Register an up/down step |
| `RegisteredMigrations` | Sorted registry snapshot |
| `Run` | Apply pending migrations to a driver database |
| `WithLogger` | Log rollback and lock release failures |
| `WithRollbackOnFailure` | Opt into rollback of all applied steps on failure |

The [root package](../README.md) runs these migrations during Connect. Register steps through this package directly. Registration should finish before running migrations. The lease uses server time; it does not fence work that ignores context cancellation.
