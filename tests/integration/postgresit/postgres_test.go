// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgresit_test

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters/memory"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/reliability/outbox"
	"github.com/altessa-s/go-atlas/store/postgres"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	outboxconfig "github.com/altessa-s/go-atlas/config/outbox"
	cfg "github.com/altessa-s/go-atlas/config/postgres"
	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	outboxfactory "github.com/altessa-s/go-atlas/factory/reliability/outbox"
	factory "github.com/altessa-s/go-atlas/factory/store/postgres"
	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
)

type healthChecks map[string]health.Checker

func (h healthChecks) RegisterService(name string, c health.Checker) { h[name] = c }

type spanSink struct {
	adapters.MultiAdapter
	spans chan adapters.SpanData
}

func (s *spanSink) ExportSpans(_ context.Context, spans []adapters.SpanData) error {
	for _, sp := range spans {
		s.spans <- sp
	}
	return nil
}

func TestPools(t *testing.T) {
	for _, tc := range []struct{ name, env, port string }{{"direct", "POSTGRES_DSN", "15432"}, {"transaction", "PGBOUNCER_DSN", "16432"}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			dsn := cmp.Or(os.Getenv(tc.env), "postgres://atlas:atlas@127.0.0.1:"+tc.port+"/atlas?sslmode=disable")
			c := cfg.Default()
			c.DSN = redacted.RedactedString(dsn)
			c.Pooling = tc.name
			c.Pools = map[string]cfg.Pool{"worker": {DSN: c.DSN, MaxOpenConnections: 2}}
			healths := healthChecks{}
			ma := memory.New()
			meter := metrics.New(metrics.WithAdapter(ma), metrics.WithServiceName("test"))
			sink := &spanSink{spans: make(chan adapters.SpanData, 32)}
			tr := tracing.New(tracing.WithAdapter(sink))
			t.Cleanup(func() { _ = tr.Shutdown(context.Background()) })
			schema := "pgit_" + strings.ToLower(rand.Text()[:12])
			probe, err := sql.Open("pgx", dsn)
			require.NoError(t, sqldialect.SafeError(nil, "open PostgreSQL preflight", err))
			probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err = probe.PingContext(probeCtx)
			cancel()
			_ = probe.Close()
			if livebackend.SkipEndpoint(err, os.Getenv(tc.env)) {
				t.Skip("compose endpoint unavailable")
			}
			require.NoError(t, sqldialect.SafeError(nil, "ping PostgreSQL preflight", err))
			cleanup, err := postgres.Open(ctx, c.DSN, postgres.WithPooling(postgres.PoolingMode(tc.name)))
			require.NoError(t, err)
			t.Cleanup(func() {
				_, err := cleanup.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
				require.NoError(t, err)
				_ = cleanup.Close()
			})

			pools, err := factory.New(&c).UseHealthCoordinator(healths).UseMeter(meter).UseTracer(tr).Build(ctx)
			require.NoError(t, err)
			t.Cleanup(func() { _ = pools.Close() })
			db := pools[postgres.DefaultPool]
			require.Equal(t, 2, pools["worker"].Stats().MaxOpenConnections)
			require.Len(t, healths, 2)
			require.Equal(t, health.StatusServing, healths["postgres.worker"].CheckHealth(ctx))
			require.Positive(t, ma.GaugeValue("test_postgres_open_connections", metrics.Labels{"pool": "worker"}))
			var version int
			require.NoError(t, db.QueryRowContext(ctx, "SHOW server_version_num").Scan(&version))
			require.Equal(t, 17, version/10000)
			table := schema + ".items"
			_, err = db.ExecContext(ctx, "CREATE SCHEMA "+schema)
			require.NoError(t, err)
			_, err = db.ExecContext(ctx, "CREATE TABLE "+table+" (id integer PRIMARY KEY)")
			require.NoError(t, err)
			events := schema + ".events_outbox"
			obConfig := &outboxconfig.Config{Enabled: true, Storage: &outboxconfig.Storage{
				Type: outboxconfig.StorageTypeSQL, SQL: &outboxconfig.SQLStorage{Dialect: outboxconfig.SQLDialectPostgres, Table: events, EnsureSchema: true},
			}}
			eventsWriter, err := outboxfactory.New(obConfig).UseSQLDB(pools["worker"]).Build(ctx, func(context.Context, outbox.Event) error { return nil })
			require.NoError(t, err)
			q := postgres.Querier(db)
			marker := errors.New("discard unit")
			calls := 0
			type requestKey struct{}
			ctx = context.WithValue(ctx, requestKey{}, "request")
			work := func(txCtx context.Context) error {
				require.True(t, postgres.InWriteTransaction(txCtx))
				outer, ok := postgres.Tx(txCtx)
				require.True(t, ok)
				require.NoError(t, postgres.WithTransaction(txCtx, db, func(inner context.Context) error {
					tx, _ := postgres.Tx(inner)
					require.Same(t, outer, tx)
					_, err := q.ExecContext(inner, "INSERT INTO "+table+" VALUES (1)")
					return err
				}))
				require.ErrorIs(t, postgres.WithSavepoint(txCtx, db, func(child context.Context) error {
					postgres.AfterCommit(child, func() { t.Error("rolled back savepoint hook ran") })
					_, err := q.ExecContext(child, "INSERT INTO "+table+" VALUES (1)")
					name, ok := postgres.UniqueViolation(err)
					require.True(t, ok)
					require.Equal(t, "items_pkey", name)
					return marker
				}), marker)
				require.NoError(t, postgres.WithSavepoint(txCtx, db, func(child context.Context) error {
					postgres.AfterCommitContext(child, func(fresh context.Context) {
						_, ok := postgres.Tx(fresh)
						require.False(t, ok)
						require.Nil(t, fresh.Done())
						require.Equal(t, "request", fresh.Value(requestKey{}))
						calls++
					})
					return nil
				}))

				require.NoError(t, eventsWriter.Save(outboxsql.WithTx(txCtx, outer), outbox.Event{Id: "one", Key: "item.created", Status: outbox.StatusPending, Payload: []byte("one")}))
				require.Equal(t, 1, count(t, txCtx, q, events))
				require.Zero(t, count(t, ctx, cleanup, events))
				require.Zero(t, calls)
				return nil
			}
			require.ErrorIs(t, postgres.WithTransaction(ctx, db, func(txCtx context.Context) error { require.NoError(t, work(txCtx)); return marker }), marker)
			require.Zero(t, count(t, ctx, db, table))
			require.Zero(t, count(t, ctx, db, events))
			require.Zero(t, calls)
			require.NoError(t, postgres.WithTransaction(ctx, db, work))
			require.Equal(t, 1, calls)
			require.Equal(t, 1, count(t, ctx, db, events))
			require.Error(t, postgres.WithSnapshot(ctx, db, func(snap context.Context) error {
				require.False(t, postgres.InWriteTransaction(snap))
				require.Equal(t, 1, count(t, snap, q, table))
				_, err := cleanup.ExecContext(ctx, "INSERT INTO "+table+" VALUES (2)")
				require.NoError(t, err)
				require.Equal(t, 1, count(t, snap, q, table))
				_, err = q.ExecContext(snap, "INSERT INTO "+table+" VALUES (3)")
				require.Error(t, err)
				return err
			}))
			require.NoError(t, tr.ForceFlush(ctx))
			require.Empty(t, sink.spans)
			traced, parent := tr.Recorder("test").Start(ctx, "parent")
			var value int
			require.NoError(t, q.QueryRowContext(traced, "-- name: ReadNumber :one\nSELECT length($1::text)", "private parameter").Scan(&value))
			parent.End()
			require.NoError(t, tr.ForceFlush(ctx))
			var client bool
			for len(sink.spans) > 0 {
				sp := <-sink.spans
				if sp.Kind == adapters.SpanKindClient {
					require.Equal(t, "ReadNumber", sp.Name)
					require.NotContains(t, fmt.Sprint(sp), "private parameter")
					client = true
				}
			}
			require.True(t, client)
			require.NoError(t, postgres.WithTransaction(ctx, db, func(txCtx context.Context) error {
				_, err := postgres.WithTransactionSettings(txCtx, db, postgres.TransactionSettings{SynchronousCommit: "local", LockTimeout: time.Second})
				if err != nil {
					return err
				}
				var setting string
				err = q.QueryRowContext(txCtx, "SHOW synchronous_commit").Scan(&setting)
				require.Equal(t, "local", setting)
				return err
			}))
			async, err := postgres.BeginAsyncCommit(ctx, db)
			require.NoError(t, err)
			var mode string
			require.NoError(t, async.QueryRowContext(ctx, "SHOW synchronous_commit").Scan(&mode))
			require.Equal(t, "off", mode)
			require.NoError(t, async.Rollback())
			testDeadConnection(t, ctx, pools["worker"], table)
			t.Run("savepoint_cancel", func(t *testing.T) { testCanceledSavepoint(t, db, table) })
			t.Run("commit_failure", func(t *testing.T) { testFailedCommit(t, db, schema) })
			t.Run("timeouts", func(t *testing.T) { testTimeouts(t, db) })

		})
	}
}

func count(t testing.TB, ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, table string) int {
	t.Helper()
	var n int
	require.NoError(t, q.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n))
	return n
}

// sever closes the client socket before the next command writes any bytes.
func sever(t *testing.T, c *sql.Conn) {
	t.Helper()
	require.NoError(t, c.Raw(func(raw any) error { return raw.(interface{ Conn() *pgx.Conn }).Conn().PgConn().Conn().Close() }))
}

func testDeadConnection(t *testing.T, ctx context.Context, db *sql.DB, table string) {
	t.Helper()
	db.SetMaxOpenConns(1)
	q := postgres.Querier(db)
	var one int
	require.NoError(t, q.QueryRowContext(ctx, "SELECT $1::int", 1).Scan(&one))
	conn, err := db.Conn(ctx)
	require.NoError(t, err)
	sever(t, conn)
	require.NoError(t, conn.Close())
	require.NoError(t, q.QueryRowContext(ctx, "SELECT $1::int", 1).Scan(&one))
	require.Equal(t, 1, one)
	conn, err = db.Conn(ctx)
	require.NoError(t, err)
	tx, err := conn.BeginTx(ctx, nil)
	require.NoError(t, err)
	sever(t, conn)
	_, err = tx.ExecContext(ctx, "INSERT INTO "+table+" VALUES (4)")
	require.Error(t, err)
	require.Error(t, tx.Commit())
	_ = conn.Close()
	require.Equal(t, 2, count(t, ctx, db, table))
}

func testCanceledSavepoint(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	q := postgres.Querier(db)
	require.NoError(t, postgres.WithTransaction(t.Context(), db, func(ctx context.Context) error {
		child, cancel := context.WithCancel(ctx)
		defer cancel()
		err := postgres.WithSavepoint(child, db, func(sp context.Context) error {
			_, err := q.ExecContext(sp, "INSERT INTO "+table+" VALUES (5)")
			require.NoError(t, err)
			cancel()
			return child.Err()
		})
		require.ErrorIs(t, err, context.Canceled)
		var has bool
		require.NoError(t, q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+table+" WHERE id = 5)").Scan(&has))
		require.False(t, has)
		return nil
	}))
}

func testFailedCommit(t *testing.T, db *sql.DB, schema string) {
	t.Helper()
	table := schema + ".deferred_items"
	_, err := db.ExecContext(t.Context(), "CREATE TABLE "+table+" (n integer UNIQUE DEFERRABLE INITIALLY DEFERRED)")
	require.NoError(t, err)
	called := false
	err = postgres.WithTransaction(t.Context(), db, func(ctx context.Context) error {
		postgres.AfterCommit(ctx, func() { called = true })
		_, err := postgres.Queryer(ctx, db).ExecContext(ctx, "INSERT INTO "+table+" VALUES (1), (1)")
		return err
	})
	name, ok := postgres.UniqueViolation(err)
	require.True(t, ok)
	require.Equal(t, "deferred_items_n_key", name)
	require.False(t, called)
	require.Zero(t, count(t, t.Context(), db, table))
}

func testTimeouts(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := t.Context()
	var before, after string
	require.NoError(t, db.QueryRowContext(ctx, "SHOW statement_timeout").Scan(&before))
	limited, err := postgres.WithTransactionSettings(ctx, db, postgres.TransactionSettings{
		StatementTimeout: 15 * time.Millisecond, IdleTransactionTimeout: time.Second,
	})
	require.NoError(t, err)
	err = postgres.WithTransaction(limited, db, func(ctx context.Context) error {
		_, err := postgres.Queryer(ctx, db).ExecContext(ctx, "SELECT pg_sleep(2)")
		return err
	})
	kind, _ := postgres.Classify(err)
	require.Equal(t, postgres.StatementTimeout, kind)
	require.NoError(t, db.QueryRowContext(ctx, "SHOW statement_timeout").Scan(&after))
	require.Equal(t, before, after)
	expired, cancel := context.WithCancel(ctx)
	cancel()
	_, err = db.ExecContext(expired, "SELECT 1")
	kind, _ = postgres.Classify(err)
	require.Equal(t, postgres.Canceled, kind)
}

func TestSkipEndpoint(t *testing.T) {
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	require.True(t, livebackend.SkipEndpoint(fmt.Errorf("connect: %w", refused), ""))
	require.False(t, livebackend.SkipEndpoint(refused, "explicit"))
	require.False(t, livebackend.SkipEndpoint(errors.New("authentication failed"), ""))
	require.False(t, livebackend.SkipEndpoint(nil, ""))
}

func TestFactory_TLSWithATrustedCA(t *testing.T) {
	t.Parallel()
	dsn, haveDSN := os.LookupEnv("POSTGRES_TLS_DSN")
	ca, haveCA := os.LookupEnv("POSTGRES_TLS_CA")
	if !haveDSN && !haveCA {
		t.Skip("POSTGRES_TLS_DSN and POSTGRES_TLS_CA are unset")
	}
	require.True(t, dsn != "" && ca != "", "both POSTGRES_TLS_DSN and POSTGRES_TLS_CA must be nonempty")
	c := cfg.Default()
	c.DSN = redacted.RedactedString(dsn)
	c.TLS = &tlsconfig.Client{CACerts: []string{ca}}
	pools, err := factory.New(&c).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pools.Close()) })
	var encrypted bool
	require.NoError(t, pools[postgres.DefaultPool].QueryRowContext(t.Context(),
		"SELECT ssl FROM pg_stat_ssl WHERE pid = pg_backend_pid()").Scan(&encrypted))
	require.True(t, encrypted)
	c.TLS.ServerName = "wrong-name.invalid"
	_, err = factory.New(&c).Build(t.Context())
	var mismatch x509.HostnameError
	require.ErrorAs(t, err, &mismatch)
}
