# postgresconfig

PostgreSQL connection and startup settings for the [pool builder](../../factory/store/postgres). Use `Default()` for programmatic construction
or load the YAML schema with go-nomos. Supply the primary DSN as a redacted secret.

```yaml
postgres:
  dsn: ${POSTGRES_DSN}
  pooling: transaction
  maxOpenConnections: 16
  maxIdleConnections: 8
  pools:
    worker:
      dsn: ${WORKER_DSN}
      maxOpenConnections: 3
      lockTimeout: 6s
```

| Field | Default | Meaning |
| --- | --- | --- |
| dsn | required | PostgreSQL URI or libpq keyword/value string |
| pooling | direct | direct or transaction |
| maxOpenConnections / maxIdleConnections | 64 / 64 | Primary pool limits |
| connectionMaxLifetime / connectionMaxIdleTime | 30m / 5m | Connection recycling |
| connectTimeout | 5s | Connection establishment |
| statementTimeout / lockTimeout / idleTransactionTimeout | 10s / 3s / 30s | Server limits |
| listingTimeout / countBudget | 3s / 50ms | Query-layer budgets. Zero count budget disables exact-count probing |
| pools | empty | Additional named connection reservations |

Each named pool requires a positive open limit. Its idle limit defaults to the smaller of this limit and the primary idle limit. An explicit zero
disables retained idle connections. The DSN, pooling mode, statement and lock timeouts inherit from the primary configuration unless overridden.
Total application capacity is the sum of all pool limits, multiplied by the number of replicas.
