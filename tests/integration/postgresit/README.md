# postgresit

The suite exercises the PostgreSQL factory, pool reservations, transactions, and migrations against PostgreSQL and PgBouncer.

Set both `POSTGRES_TLS_DSN` and `POSTGRES_TLS_CA` to run `TestFactory_TLSWithATrustedCA`. It checks encryption on the active server session and
rejects a hostname mismatch. The [integration guide](../README.md#postgresql-pools) includes an offline Compose fixture and test commands.
