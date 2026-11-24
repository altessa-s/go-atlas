# migrations

```go
import "github.com/altessa-s/go-atlas/store/postgres/migrations"
```

Registry of Go schema migrations for PostgreSQL databases used through [`store/postgres`](..), applied with
[goose](https://github.com/pressly/goose) under a session-level advisory lock. A module of its own
(`github.com/altessa-s/go-atlas/store/postgres/migrations`): goose's `go.mod` requires the drivers of every database it supports, which must
not enter the dependency graph of go-atlas consumers — see the [ADR](../../../docs/adr/0001-postgres-migrations-module.md).

## Key types

| Symbol                                          | Description                                                                                         |
|-------------------------------------------------|-----------------------------------------------------------------------------------------------------|
| `Register(version, description, up, down)`      | Registers a migration; `ErrInvalidMigration` for a version below 1, a nil up or a duplicate        |
| `MustRegister(...)`                             | `Register` that panics, for `init` functions                                                        |
| `Registered()`                                  | Copy of the registry, ordered by version                                                            |
| `Run(ctx, db, opts...)`                         | Applies the pending migrations under the advisory lock; no-op with nothing registered               |
| `Down(ctx, db, opts...)`                        | Reverts the latest applied migration under the lock; `ErrIrreversibleMigration` if its down is nil  |
| `Status(ctx, db, opts...)`                      | `[]MigrationStatus` (version, description, applied, applied at); never waits for the lock           |
| `MigrationFunc`                                 | `func(ctx context.Context, tx *sql.Tx) error`; runs in the transaction that records the version     |

## Options

| Option            | Default            | Description                                                                         |
|-------------------|--------------------|-------------------------------------------------------------------------------------|
| `WithLogger`      | discard            | goose migration progress                                                    |
| `WithTableName`   | `goose_db_version` | Version table                                                                       |
| `WithLockID`      | goose's default    | Advisory lock key; every process migrating the database must use the same           |
| `WithLockTimeout` | `30s`              | Deadline for checkout, identity check and lock acquisition |

## Usage

```go
func init() {
	migrations.MustRegister(2026112301, "create items", func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TABLE items (id uuid PRIMARY KEY)`)
		return err
	}, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DROP TABLE items`)
		return err
	})
}

// At startup, through the factory:
pools, err := pgfactory.New(&cfg.Postgres).
	UseMigrator(func(ctx context.Context, db *sql.DB) error { return migrations.Run(ctx, db) }).
	Build(ctx)
```

Each run builds its own `goose.Provider` with the global registry disabled. A pending migration below the highest applied version fails the run (no
out-of-order application).

Supply a direct pgx-backed pool. The runner verifies the session with `postgres.CheckDirect` (comparing the startup backend PID with the session’s
reported PID) before taking the advisory lock; a pooler mismatch matches `ErrDirectConnectionRequired`. The lock budget covers checkout and
acquisition. When the pool factory supplies the context, `postgres.migrationTimeout` (default `30s`) bounds both acquisition and migration work, while
an earlier caller deadline takes precedence. `Down` succeeds when nothing is applied. `Status` reads the ledger in one ordered query and reports a zero
time for a missing timestamp. Ledger names use one or two lowercase identifier components of at most 63 bytes each; invalid names match
`ErrInvalidTableName`.
