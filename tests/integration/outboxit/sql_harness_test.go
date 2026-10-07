// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/tests/integration/outboxit"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"

	outboxsql "github.com/altessa-s/go-atlas/data/outbox/storages/sqldb"
)

// mysqlTimeLayout matches the DATE_FORMAT used to read DATETIME(6) columns.
const mysqlTimeLayout = "2006-01-02 15:04:05.000000"

// sqlSpec names one SQL backend the scenarios run against.
type sqlSpec struct {
	name    string
	driver  string
	dsn     string
	dialect outboxsql.Dialect
}

// sqlSpecs are the SQL backends from tests/integration/docker-compose.yml. The
// MySQL DSN deliberately sets a non-UTC loc with parseTime: the store must not
// let the driver shift any instant.
func sqlSpecs() []sqlSpec {
	return []sqlSpec{
		{"postgres", "pgx", envOr("POSTGRES_DSN", "postgres://atlas:atlas@127.0.0.1:15432/atlas?sslmode=disable"), outboxsql.DialectPostgres},
		{"mariadb", "mysql", envOr("MARIADB_DSN", "atlas:atlas@tcp(127.0.0.1:13306)/atlas"), outboxsql.DialectMySQL},
		{"mysql", "mysql", envOr("MYSQL_DSN", "atlas:atlas@tcp(127.0.0.1:13307)/atlas?parseTime=true&loc=Asia%2FTokyo"), outboxsql.DialectMySQL},
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// forEachBackend runs a scenario against MongoDB and every SQL backend, each in
// its own parallel subtest over its own fixture.
func forEachBackend(t *testing.T, scenario func(t *testing.T, f *fixture)) {
	t.Helper()
	t.Run("mongo", func(t *testing.T) {
		t.Parallel()
		scenario(t, newFixture(t))
	})
	for _, spec := range sqlSpecs() {
		t.Run(spec.name, func(t *testing.T) {
			t.Parallel()
			scenario(t, newSQLFixture(t, spec))
		})
	}
}

// sqlBackend is the SQL side of a fixture: the events table the store writes,
// a business table sharing its transactions, and readers for assertions.
type sqlBackend struct {
	db      *sql.DB
	dialect outboxsql.Dialect
	events  string
	orders  string
}

// sqlTxKey carries the business transaction to insertOrder.
type sqlTxKey struct{}

// newSQLFixture gives the test throwaway events and orders tables, dropped on
// cleanup, and a store over them whose schema EnsureSchema created.
func newSQLFixture(tb testing.TB, spec sqlSpec) *fixture {
	tb.Helper()

	b := newSQLFixtureWithoutSchema(tb, spec)
	store, err := outboxsql.New(b.db, spec.dialect, outboxsql.WithTableName(b.events))
	require.NoError(tb, err)
	// Twice, to prove idempotence.
	require.NoError(tb, store.EnsureSchema(tb.Context()))
	require.NoError(tb, store.EnsureSchema(tb.Context()))

	return &fixture{store: store, recorder: outboxit.NewRecorder(), sql: b}
}

// newSQLFixtureWithoutSchema connects to the backend and names throwaway
// events and orders tables, dropped on cleanup; only the orders table is
// created. It skips when the server is unreachable; the DSN is not printed, as
// an override may carry a password.
func newSQLFixtureWithoutSchema(tb testing.TB, spec sqlSpec) *sqlBackend {
	tb.Helper()

	db, err := sql.Open(spec.driver, spec.dsn)
	require.NoError(tb, err)
	ctx, cancel := context.WithTimeout(tb.Context(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		tb.Skipf("%s unreachable (%v) — start it with: make integration-up", spec.name, err)
	}

	// Lower case and short: the DROP below is unquoted (PostgreSQL folds it),
	// and the random part keeps parallel processes apart.
	base := "obit_" + strings.ToLower(rand.Text()[:10])
	b := &sqlBackend{db: db, dialect: spec.dialect, events: base + "_events", orders: base + "_orders"}
	tb.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+b.events)
		_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+b.orders)
		_ = db.Close()
	})

	_, err = db.ExecContext(tb.Context(), "CREATE TABLE "+b.orders+" (id VARCHAR(64) PRIMARY KEY, total INT NOT NULL)")
	require.NoError(tb, err)
	return b
}

func (b *sqlBackend) bind(query string) string {
	if b.dialect != outboxsql.DialectPostgres {
		return query
	}
	var out strings.Builder
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			out.WriteString("$" + strconv.Itoa(n))
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

func (b *sqlBackend) inTx(tb testing.TB, fn func(context.Context) error) error {
	tb.Helper()
	tx, err := b.db.BeginTx(tb.Context(), nil)
	require.NoError(tb, err)
	ctx := context.WithValue(outboxsql.WithTx(tb.Context(), tx), sqlTxKey{}, tx)
	if err := fn(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (b *sqlBackend) insertOrder(txCtx context.Context, id string, total int) error {
	tx, _ := txCtx.Value(sqlTxKey{}).(*sql.Tx)
	if tx == nil {
		return sql.ErrTxDone
	}
	_, err := tx.ExecContext(txCtx, b.bind("INSERT INTO "+b.orders+" (id, total) VALUES (?, ?)"), id, total)
	return err
}

func (b *sqlBackend) count(tb testing.TB, table string) int64 {
	tb.Helper()
	var n int64
	require.NoError(tb, b.db.QueryRowContext(tb.Context(), "SELECT COUNT(*) FROM "+table).Scan(&n))
	return n
}

// timeCol reads a timestamp in a driver-independent form: DATE_FORMAT strings
// on MySQL (whatever the DSN's loc), native values on PostgreSQL.
func (b *sqlBackend) timeCol(col string) string {
	if b.dialect == outboxsql.DialectPostgres {
		return col
	}
	return "DATE_FORMAT(" + col + ", '%Y-%m-%d %H:%i:%s.%f')"
}

func (b *sqlBackend) selectEvents(where string) string {
	return "SELECT id, status, attempts, topic, error, " + b.timeCol("created_at") + ", " + b.timeCol("published_at") + ", " +
		b.timeCol("last_attempt_on") + ", " + b.timeCol("next_attempt_at") + ", " + b.timeCol("locked_on") + ", lock_token FROM " +
		b.events + where
}

func (b *sqlBackend) scan(tb testing.TB, row interface{ Scan(...any) error }) storedEvent {
	tb.Helper()
	var (
		ev                                  storedEvent
		id, topic, lastErr, token           []byte
		created, published, attempted, next sql.Null[any]
		locked                              sql.Null[any]
	)
	require.NoError(tb, row.Scan(&id, &ev.Status, &ev.Attempts, &topic, &lastErr, &created, &published, &attempted, &next, &locked, &token))
	ev.ID, ev.Topic, ev.LockToken = string(id), string(topic), string(token)
	if lastErr != nil {
		msg := string(lastErr)
		ev.LastError = &msg
	}
	ev.CreatedAt = derefTime(b.parseTime(tb, created))
	ev.PublishedAt = b.parseTime(tb, published)
	ev.LastAttemptOn = b.parseTime(tb, attempted)
	ev.NextAttemptAt = b.parseTime(tb, next)
	ev.LockedOn = b.parseTime(tb, locked)
	return ev
}

func (b *sqlBackend) parseTime(tb testing.TB, v sql.Null[any]) *time.Time {
	tb.Helper()
	if !v.Valid {
		return nil
	}
	switch x := v.V.(type) {
	case time.Time:
		u := x.UTC()
		return &u
	case []byte:
		t, err := time.ParseInLocation(mysqlTimeLayout, string(x), time.UTC)
		require.NoError(tb, err)
		return &t
	case string:
		t, err := time.ParseInLocation(mysqlTimeLayout, x, time.UTC)
		require.NoError(tb, err)
		return &t
	default:
		tb.Fatalf("unexpected timestamp type %T", v.V)
		return nil
	}
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func (b *sqlBackend) load(tb testing.TB, id string) storedEvent {
	tb.Helper()
	return b.scan(tb, b.db.QueryRowContext(tb.Context(), b.bind(b.selectEvents(" WHERE id = ?")), id))
}

func (b *sqlBackend) loadAll(tb testing.TB) []storedEvent {
	tb.Helper()
	rows, err := b.db.QueryContext(tb.Context(), b.selectEvents(" ORDER BY created_at, seq"))
	require.NoError(tb, err)
	defer func() { _ = rows.Close() }()
	var out []storedEvent
	for rows.Next() {
		out = append(out, b.scan(tb, rows))
	}
	require.NoError(tb, rows.Err())
	return out
}
