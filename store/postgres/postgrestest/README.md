# postgrestest

`New(t)` creates a database for one test, opens a `*sql.DB` through `store/postgres.Open` and registers cleanup. Set `POSTGRES_DSN` to a direct
PostgreSQL endpoint on a dedicated test cluster. The role needs `CREATEDB` and access to the `postgres` maintenance database. An empty variable skips
the test. A configured endpoint that fails readiness verification fails the test.

```go
db := postgrestest.New(t, postgrestest.WithDatabasePrefix("orders_it_"))
err := postgres.Migrate(t.Context(), db, migrations, "orders")
```

| Option | Default | Purpose |
|---|---|---|
| `WithEnv` | `POSTGRES_DSN` | Environment variable containing a pgx URL or keyword/value DSN |
| `WithDatabasePrefix` | `it_` | Lowercase identifier prefix for generated names |
| `WithReadinessTimeout` | `1m` | Maximum wait for the maintenance endpoint |
| `WithStaleAge` | `2h` | Minimum age for a sweep candidate, using the server clock. Zero disables sweeping. |

Use the same primary server for all connections. The fixture verifies backend identity before acquiring session locks and checks the working database
before returning. A dedicated maintenance session holds the database name lock through creation and teardown. A second session holds a lock inside the
test database. Losing either session fails the test and closes the working pool.

Each sweep compares the original database OID and complete ownership comment under the name lock and checks for sessions and advisory locks. Cleanup
uses the same checks and a bounded context. A failed creation or unconfirmed marker leaves the database for inspection. Restrict database-management
privileges to cooperating fixture users. A role with independent DDL privileges can bypass advisory locks.
