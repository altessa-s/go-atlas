// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgresit_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/store/postgres/postgrestest"
)

// recordingTB records what postgrestest reports; Fatalf and Skipf end the calling goroutine like testing.T does.
type recordingTB struct {
	testing.TB
	mu   sync.Mutex
	msgs []string
}

func (r *recordingTB) Helper() {}

func (r *recordingTB) record(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, fmt.Sprintf(format, args...))
}

func (r *recordingTB) Logf(format string, args ...any) { r.record(format, args...) }

func (r *recordingTB) Fatalf(format string, args ...any) {
	r.record(format, args...)
	runtime.Goexit()
}

func (r *recordingTB) Skipf(format string, args ...any) {
	r.record(format, args...)
	runtime.Goexit()
}

func (r *recordingTB) output() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.msgs, "\n")
}

func (r *recordingTB) Errorf(format string, args ...any) { r.record(format, args...) }

func fixturePrefix() string { return "pgit_" + strings.ToLower(rand.Text()[:12]) + "_" }

func fixtureDSN(tb testing.TB) string {
	tb.Helper()
	dsn := strings.TrimSpace(os.Getenv(postgrestest.DefaultEnv))
	if dsn == "" {
		tb.Skip("POSTGRES_DSN not configured")
	}
	if scheme, rest, ok := strings.Cut(dsn, "://"); ok && (strings.EqualFold(scheme, "postgres") || strings.EqualFold(scheme, "postgresql")) {
		return strings.ToLower(scheme) + "://" + rest
	}
	return dsn
}

func openDatabase(tb testing.TB, name string) *sql.DB {
	tb.Helper()
	cfg, err := pgx.ParseConfig(fixtureDSN(tb))
	require.NoError(tb, sqldialect.SafeError(nil, "parse test DSN", err))
	cfg.Database = name
	db := stdlib.OpenDB(*cfg)
	tb.Cleanup(func() { require.NoError(tb, sqldialect.SafeError(nil, "close test pool", db.Close())) })
	require.NoError(tb, sqldialect.SafeError(nil, "ping test database", db.PingContext(tb.Context())))
	return db
}

func databaseNames(ctx context.Context, admin *sql.DB, prefix string) ([]string, error) {
	rows, err := admin.QueryContext(ctx, "SELECT datname FROM pg_database WHERE starts_with(datname,$1) ORDER BY datname", prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func cleanupDatabase(tb testing.TB, admin *sql.DB, name string) {
	tb.Helper()
	tb.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(tb.Context()), 10*time.Second)
		defer cancel()
		_, err := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		require.NoError(tb, sqldialect.SafeError(nil, "remove suite database", err))
	})
}

// runNew calls postgrestest.New with a recording TB on its own goroutine, so a Fatalf ends it rather than the test.
func runNew(t testing.TB, opts ...postgrestest.Option) string {
	t.Helper()
	tb := &recordingTB{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		postgrestest.New(tb, opts...)
	}()
	<-done
	return tb.output()
}

// No driver error text reaches the test output, whatever the DSN form; pgx quotes the DSN
// of a parse error and misses some password spellings. Serial: t.Setenv sets the DSN variable.
func TestPostgrestest_DriverErrorsHideThePassword(t *testing.T) { //nolint:paralleltest // t.Setenv
	for _, tc := range testhelpers.PostgresSecretDSNs() {
		t.Run(tc.Name, func(t *testing.T) {
			t.Setenv("PGIT_BAD_DSN", tc.DSN)
			out := runNew(t, postgrestest.WithEnv("PGIT_BAD_DSN"), postgrestest.WithReadinessTimeout(50*time.Millisecond))
			require.True(t, strings.Contains(out, "parse DSN failed") || strings.Contains(out, "verify direct configured endpoint failed"), "missing parse or connect failure")
			for _, s := range tc.Secrets {
				require.False(t, strings.Contains(out, s), "password in fixture diagnostic")
			}
		})
	}
}

// A dbname query parameter of the DSN must not send the test to the database it names. Serial: t.Setenv.
func TestPostgrestest_DatabaseParameterIsReplaced(t *testing.T) { //nolint:paralleltest // t.Setenv
	dsn := fixtureDSN(t)
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		dsn += " dbname=postgres"
	} else {
		u, err := url.Parse(dsn)
		require.NoError(t, sqldialect.SafeError(nil, "parse test URL", err))
		q := u.Query()
		q.Set("dbname", "postgres")
		u.RawQuery = q.Encode()
		dsn = u.String()
	}
	t.Setenv("PGIT_DBNAME_DSN", dsn)
	prefix := fixturePrefix()

	pg := postgrestest.New(t, postgrestest.WithEnv("PGIT_DBNAME_DSN"), postgrestest.WithDatabasePrefix(prefix))
	var name string
	require.NoError(t, pg.QueryRowContext(t.Context(), `SELECT current_database()`).Scan(&name))
	require.True(t, strings.HasPrefix(name, prefix), "the test runs in its throwaway database, not %q", name)
}

// The advisory lock that marks a database as in use is held outside the returned pool, so a test may
// size that pool down to one connection.
func TestPostgrestest_LockDoesNotTakeThePoolSlot(t *testing.T) {
	t.Parallel()
	fixtureDSN(t)

	pg := postgrestest.New(t, postgrestest.WithDatabasePrefix(fixturePrefix()))
	pg.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	var one int
	require.NoError(t, pg.QueryRowContext(ctx, `SELECT 1`).Scan(&one), "the only connection of the pool is free")
}

// setupObserver pauses creation synchronously after the marked database acquires its cleanup.
type setupObserver struct {
	testing.TB
	registrations int
	observe       func()
}

func (o *setupObserver) Cleanup(f func()) {
	o.TB.Cleanup(f)
	o.registrations++
	if o.registrations == 2 {
		o.observe()
	}
}

func TestPostgrestest_StartingDatabasesAreNeverSweepable(t *testing.T) {
	t.Parallel()
	admin := openDatabase(t, "postgres")
	prefix := fixturePrefix()
	var seen string
	tb := &setupObserver{TB: t, observe: func() {
		names, err := databaseNames(t.Context(), admin, prefix)
		require.NoError(t, err)
		require.Len(t, names, 1)
		seen = names[0]
		var marker string
		var idle bool
		err = admin.QueryRowContext(t.Context(), `SELECT shobj_description(oid,'pg_database'),
  NOT EXISTS (SELECT FROM pg_stat_activity WHERE datid=d.oid) FROM pg_database d WHERE datname=$1`, seen).Scan(&marker, &idle)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(marker, postgrestest.Marker))
		require.True(t, idle)
		// At this instant the name lock alone protects the creator. Force a second New to scan this candidate.
		other := postgrestest.New(t, postgrestest.WithDatabasePrefix(prefix), postgrestest.WithStaleAge(time.Nanosecond))
		require.NoError(t, other.PingContext(t.Context()))
	}}
	db := postgrestest.New(tb, postgrestest.WithDatabasePrefix(prefix))
	var actual string
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT current_database()").Scan(&actual))
	require.NotEmpty(t, seen)
	require.Equal(t, seen, actual)
}

// The sweep drops only stale databases postgrestest created (its marker comment) and no live process holds
// (its advisory lock); a database merely named like one, or one still in use, survives.
func TestPostgrestest_SweepDropsOnlyAbandonedMarkedDatabases(t *testing.T) {
	t.Parallel()
	admin := openDatabase(t, "postgres")
	prefix := fixturePrefix()
	old := time.Now().Add(-3 * time.Hour).Unix()
	unmarked := fmt.Sprintf("%s%08x_00000001", prefix, old)
	abandoned := fmt.Sprintf("%s%08x_00000002", prefix, old)
	live := fmt.Sprintf("%s%08x_00000003", prefix, old)
	for _, name := range []string{unmarked, abandoned, live} {
		_, err := admin.ExecContext(t.Context(), `CREATE DATABASE "`+name+`"`)
		require.NoError(t, err)
		cleanupDatabase(t, admin, name)
	}
	for _, name := range []string{abandoned, live} {
		_, err := admin.ExecContext(t.Context(), `COMMENT ON DATABASE "`+name+`" IS '`+postgrestest.Marker+`'`)
		require.NoError(t, err)
	}
	liveDB := openDatabase(t, live)
	holder, err := liveDB.Conn(t.Context())
	require.NoError(t, err)
	// The session is active even though its advisory lock has disappeared.
	_, err = holder.ExecContext(t.Context(), `SELECT 1`)
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Close() })

	postgrestest.New(t, postgrestest.WithDatabasePrefix(prefix), postgrestest.WithStaleAge(time.Hour))

	exists := func(name string) bool {
		var ok bool
		require.NoError(t, admin.QueryRowContext(t.Context(), `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, name).Scan(&ok))
		return ok
	}
	require.True(t, exists(unmarked), "a database without the marker is never dropped")
	require.True(t, exists(live), "a database a live process holds is never dropped")
	require.False(t, exists(abandoned), "an abandoned stale database of the helper is dropped")
}

// Replace the database after its pool and lease have closed but before its registered drop.
// Even copying the exact marker must not authorize deletion of the new OID.
func TestPostgrestest_CleanupRechecksIdentity(t *testing.T) {
	t.Parallel()
	admin := openDatabase(t, "postgres")
	prefix := fixturePrefix()
	var name string
	var output *recordingTB
	t.Run("owner", func(t *testing.T) {
		output = &recordingTB{TB: t}
		tb := &setupObserver{TB: output, observe: func() {
			names, err := databaseNames(t.Context(), admin, prefix)
			require.NoError(t, err)
			require.Len(t, names, 1)
			name = names[0]
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
				defer cancel()
				var marker string
				require.NoError(t, admin.QueryRowContext(ctx, "SELECT shobj_description(oid,'pg_database') FROM pg_database WHERE datname=$1", name).Scan(&marker))
				ident := pgx.Identifier{name}.Sanitize()
				_, err := admin.ExecContext(ctx, "DROP DATABASE "+ident+" WITH (FORCE)")
				require.NoError(t, err)
				_, err = admin.ExecContext(ctx, "CREATE DATABASE "+ident)
				require.NoError(t, err)
				_, err = admin.ExecContext(ctx, "COMMENT ON DATABASE "+ident+" IS '"+strings.ReplaceAll(marker, "'", "''")+"'")
				require.NoError(t, err)
			})
		}}
		db := postgrestest.New(tb, postgrestest.WithDatabasePrefix(prefix))
		require.NoError(t, db.PingContext(t.Context()))
	})
	require.NotEmpty(t, name)
	cleanupDatabase(t, admin, name)
	names, err := databaseNames(t.Context(), admin, prefix)
	require.NoError(t, err)
	require.Equal(t, []string{name}, names)
	require.Contains(t, output.output(), "drop database "+name+" failed")
}
