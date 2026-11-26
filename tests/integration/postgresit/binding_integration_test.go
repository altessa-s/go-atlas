// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgresit_test

import (
	"cmp"
	"context"
	"crypto/rand"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/reliability/outbox"
	"github.com/altessa-s/go-atlas/store/postgres"
	"github.com/altessa-s/go-atlas/store/postgres/postgrestest"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	cfg "github.com/altessa-s/go-atlas/config/postgres"
	factory "github.com/altessa-s/go-atlas/factory/store/postgres"
	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
)

func TestPoolsBinding_DirectTwoDatabasesMismatch(t *testing.T) {
	t.Parallel()

	db1 := postgrestest.New(t)
	db2 := postgrestest.New(t)

	pools := postgres.Pools{
		postgres.DefaultPool: db1,
		"reporting":          db2,
	}

	boundDB, resolve, err := pools.Binding(t.Context(), "reporting")
	require.NoError(t, err)
	events, err := outboxsql.New(boundDB, outboxsql.DialectPostgres,
		outboxsql.WithTxResolver(resolve),
		outboxsql.WithTableName("events_outbox"),
	)
	require.NoError(t, err)
	require.NoError(t, events.EnsureSchema(t.Context()))

	err = postgres.WithTransaction(t.Context(), db1, func(txCtx context.Context) error {
		return events.SaveEvents(txCtx, outbox.Event{
			Id:      "ev-1",
			Key:     "order.created",
			Payload: []byte("payload"),
			Status:  outbox.StatusPending,
		})
	})
	require.ErrorIs(t, err, postgres.ErrDatabaseMismatch)
	require.Zero(t, count(t, t.Context(), db2, "events_outbox"), "bound store must not write outside the transaction")
}

func TestPoolsBinding_PgBouncerAndDirectPoolSameDatabase(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	directDSN := cmp.Or(os.Getenv("POSTGRES_DSN"), "postgres://atlas:atlas@127.0.0.1:15432/atlas?sslmode=disable")
	bouncerDSN := cmp.Or(os.Getenv("PGBOUNCER_DSN"), "postgres://atlas:atlas@127.0.0.1:16432/atlas?sslmode=disable")

	for _, ep := range []struct{ dsn, env string }{{directDSN, os.Getenv("POSTGRES_DSN")}, {bouncerDSN, os.Getenv("PGBOUNCER_DSN")}} {
		probe, err := sql.Open("pgx", ep.dsn)
		require.NoError(t, sqldialect.SafeError(nil, "open PostgreSQL preflight", err))
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = probe.PingContext(probeCtx)
		cancel()
		_ = probe.Close()
		if livebackend.SkipEndpoint(err, ep.env) {
			t.Skip("compose endpoint unavailable")
		}
		require.NoError(t, sqldialect.SafeError(nil, "ping PostgreSQL preflight", err))
	}

	directDB, err := postgres.Open(ctx, redacted.RedactedString(directDSN), postgres.WithPooling(postgres.PoolingDirect))
	require.NoError(t, err)
	t.Cleanup(func() { _ = directDB.Close() })

	pooledDB, err := postgres.Open(ctx, redacted.RedactedString(bouncerDSN), postgres.WithPooling(postgres.PoolingTransaction))
	require.NoError(t, err)
	t.Cleanup(func() { _ = pooledDB.Close() })

	pools := postgres.Pools{
		postgres.DefaultPool: directDB,
		"pooled":             pooledDB,
	}
	require.NoError(t, pools.Verify(ctx))

	schema := "pgbind_" + strings.ToLower(rand.Text()[:12])
	_, err = directDB.ExecContext(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = directDB.ExecContext(context.WithoutCancel(t.Context()), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	})

	table := schema + ".events_outbox"
	boundDB, resolve, err := pools.Binding(ctx, "pooled")
	require.NoError(t, err)
	events, err := outboxsql.New(boundDB, outboxsql.DialectPostgres,
		outboxsql.WithTxResolver(resolve),
		outboxsql.WithTableName(table),
	)
	require.NoError(t, err)
	require.NoError(t, events.EnsureSchema(ctx))

	require.NoError(t, postgres.WithTransaction(ctx, directDB, func(txCtx context.Context) error {
		return events.SaveEvents(txCtx, outbox.Event{
			Id:      "ev-1",
			Key:     "order.created",
			Payload: []byte("payload"),
			Status:  outbox.StatusPending,
		})
	}))
	require.Equal(t, 1, count(t, ctx, pooledDB, table))
}

func TestFactoryBuild_VerifyRejectsMismatchedDatabase(t *testing.T) {
	t.Parallel()

	db1 := postgrestest.New(t)
	db2 := postgrestest.New(t)

	var name1, name2 string
	require.NoError(t, db1.QueryRowContext(t.Context(), "SELECT current_database()").Scan(&name1))
	require.NoError(t, db2.QueryRowContext(t.Context(), "SELECT current_database()").Scan(&name2))

	baseDSN := strings.TrimSpace(os.Getenv("POSTGRES_DSN"))
	dsnWithDB := func(name string) redacted.RedactedString {
		if strings.HasPrefix(strings.ToLower(baseDSN), "postgres://") || strings.HasPrefix(strings.ToLower(baseDSN), "postgresql://") {
			u, err := url.Parse(baseDSN)
			require.NoError(t, sqldialect.SafeError(nil, "parse POSTGRES_DSN", err))
			u.Path, u.RawPath = "/"+name, ""
			q := u.Query()
			q.Del("dbname")
			q.Del("database")
			u.RawQuery = q.Encode()
			return redacted.RedactedString(u.String())
		}
		return redacted.RedactedString(baseDSN + " dbname=" + name)
	}

	c := cfg.Default()
	c.DSN = dsnWithDB(name1)
	c.Pools = map[string]cfg.Pool{
		"other": {DSN: dsnWithDB(name2), MaxOpenConnections: 2},
	}

	_, err := factory.New(&c).Build(t.Context())
	require.ErrorIs(t, err, postgres.ErrDatabaseMismatch)
}
