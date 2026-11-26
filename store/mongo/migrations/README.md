# migrations

```go
import "github.com/altessa-s/go-atlas/store/mongo/migrations"
```

Registers and runs versioned schema migrations. Uses the existing mongo-migrate record format and a separate database lease collection.

## API

| Symbol | Purpose |
|--------|---------|
| `RegisterMigration` | Register a step with an up, a down, and optional cleanup |
| `Registry.Register` | Add a step to an independent, zero-value-ready registry |
| `WithCleanupOnFailure` | Compensate for partial changes if this step's up returns an error |
| `WithRegistry` | Apply only the supplied registry; omit to merge the default and legacy registrations |
| `RegisteredMigrations` | Sorted registry snapshot |
| `Run` | Apply pending migrations to a driver database |
| `WithLogger` | Log rollback and lock release failures |
| `WithRollbackOnFailure` | Opt into rollback of the steps applied by this run on failure |

The [root package](../README.md) runs these migrations during Connect. Register steps through this package directly. Registration should finish before running migrations. The lease uses server time and is renewed every third of a lease until the work returns, so a step that ignores context cancellation does not overlap a successor while renewals succeed; a failed renewal cancels the work and skips rollback. A step registered with a nil down is irreversible (`ErrIrreversibleMigration`).

## Cleanup after a failed step

```go
err := registry.Register(5, "backfill order status", backfillStatus, revertStatus,
    migrations.WithCleanupOnFailure(cleanupPartialBackfill))
```

Cleanup runs only for the step whose up returns an error. It runs before any rollback of earlier steps, with a fresh `MigrationTimeout` budget
after the runner verifies lease ownership. Cleanup still runs after cancellation of the up context if the runner can verify the lease. The cleanup
context is canceled if the lease is lost. The runner skips cleanup if it cannot verify the lease and returns that failure with the up error.

Write cleanup to handle every state the up can leave behind, including no changes. Make it safe to repeat across retries, limit it to changes from
this step, and honor context cancellation. A step without a cleanup handler may leave partial changes for a later retry or manual repair.
The failed step remains pending in the ledger and its down is never called. Cleanup errors are joined with the up error, so callers can match both
with `errors.Is`. Optional rollback still handles earlier steps applied by this run. A successful up followed by a ledger error follows ledger
reconciliation, and a panic propagates without invoking cleanup.
