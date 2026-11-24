# ADR 0001. PostgreSQL migrations use goose in a separate Go module

Accepted on 2026-11-24.

## Context

Goose supports multiple SQL backends. Its module graph includes drivers and SDKs for those backends, while the PostgreSQL runtime and pool factory
need only pgx. Applications that use the runtime can manage their own startup lifecycle.

## Decision

`store/postgres/migrations` owns PostgreSQL migrations. Each application registers versioned Go callbacks. Goose records each version in the transaction
that runs the callback. The runner requires a direct connection because its advisory lock belongs to one PostgreSQL session across transactions.

The migrations module depends on the root for connection identity and safe diagnostics. The root factory accepts a
`func(context.Context, *sql.DB) error` through `UseMigrator`. The application supplies the callback and imports the migrations module when needed.

## Consequences

The migrations module has its own release tags, module checksums and optgen tool directive. The root dependency graph stays free of goose. Integration
tests resolve both repository modules through local replacements. The architecture suite checks that only the migrations module imports goose.
