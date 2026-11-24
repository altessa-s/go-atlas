# Irreversible PostgreSQL migrations

This suite registers irreversible migrations in its own test binary. Set `POSTGRES_DSN` to a direct test database and run `go test -race -count=3`.
