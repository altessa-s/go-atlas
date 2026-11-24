// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgresit_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/store/postgres/postgrestest"
)

func TestPostgrestest_PanicOnTheLockConnectionNeverBlocks(t *testing.T) {
	t.Parallel()
	testPanicCleanup(t, 3)
}

func TestPostgrestest_RejectsTransactionPooler(t *testing.T) {
	t.Parallel()
	if os.Getenv("PGBOUNCER_DSN") == "" {
		t.Skip("PGBOUNCER_DSN not configured")
	}
	// Verify that these credentials actually reach the supplied endpoint.
	conn, err := pgx.Connect(t.Context(), os.Getenv("PGBOUNCER_DSN"))
	require.NoError(t, sqldialect.SafeError(nil, "connect to configured pooler", err))
	defer func() { require.NoError(t, conn.Close(t.Context())) }()
	var backend uint32
	require.NoError(t, conn.QueryRow(t.Context(), "SELECT pg_backend_pid()").Scan(&backend))
	require.NotEqual(t, backend, conn.PgConn().PID(), "test endpoint must virtualize backend sessions")
	out := runNew(t, postgrestest.WithEnv("PGBOUNCER_DSN"), postgrestest.WithDatabasePrefix(fixturePrefix()), postgrestest.WithReadinessTimeout(3*time.Second))
	require.Regexp(t, "verify direct (configured endpoint|maintenance connection) failed", out)
}

func TestPostgrestest_ReportsLostLease(t *testing.T) {
	t.Parallel()
	admin := openDatabase(t, "postgres")
	prefix := fixturePrefix()
	tb := &recordingTB{TB: t}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		names, err := databaseNames(ctx, admin, prefix)
		require.NoError(t, err)
		require.Empty(t, names)
		require.NotContains(t, tb.output(), "drop database", "teardown unexpectedly failed")
	})
	db := postgrestest.New(tb, postgrestest.WithDatabasePrefix(prefix))
	var name string
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT current_database()").Scan(&name))
	var terminated bool
	require.NoError(t, admin.QueryRowContext(t.Context(), `SELECT pg_terminate_backend(pid) FROM pg_locks
 WHERE locktype='advisory' AND database=(SELECT oid FROM pg_database WHERE datname=$1) AND granted`, name).Scan(&terminated))
	require.True(t, terminated)
	require.Eventually(t, func() bool { return tb.output() != "" }, 10*time.Second, 20*time.Millisecond)
	require.Contains(t, tb.output(), "verify database lease failed")
	postgrestest.New(t, postgrestest.WithDatabasePrefix(prefix), postgrestest.WithStaleAge(time.Nanosecond))
	names, err := databaseNames(t.Context(), admin, prefix)
	require.NoError(t, err)
	require.Contains(t, names, name)
}
