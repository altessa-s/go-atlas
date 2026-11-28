// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package migrations

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// isolateRegistry empties the process-wide migration registry for the test and restores it afterwards. Tests using it must not run
// in parallel: the registry is global.
func isolateRegistry(tb testing.TB) {
	tb.Helper()
	registry.mu.Lock()
	saved := registry.migrations
	registry.migrations = nil
	registry.mu.Unlock()
	tb.Cleanup(func() {
		registry.mu.Lock()
		registry.migrations = saved
		registry.mu.Unlock()
	})
}

func noop(context.Context, *sql.Tx) error { return nil }

// TestRegister is serial: it swaps the process-wide migration registry.
func TestRegister(t *testing.T) { //nolint:paralleltest // swaps the process-wide migration registry
	isolateRegistry(t)

	require.ErrorIs(t, Register(0, "zero", noop, nil), ErrInvalidMigration)
	require.ErrorIs(t, Register(-1, "negative", noop, nil), ErrInvalidMigration)
	require.ErrorIs(t, Register(3, "no up", nil, nil), ErrInvalidMigration)

	require.NoError(t, Register(2026112303, "third", noop, nil))
	require.NoError(t, Register(2026112301, "first", noop, noop))
	require.ErrorIs(t, Register(2026112303, "again", noop, nil), ErrInvalidMigration)
	require.Panics(t, func() { MustRegister(2026112301, "again", noop, nil) })
	require.NotPanics(t, func() { MustRegister(2026112302, "second", noop, nil) })

	got := Registered()
	require.Len(t, got, 3)
	require.Equal(t, []int64{2026112301, 2026112302, 2026112303}, []int64{got[0].Version, got[1].Version, got[2].Version})
	require.Equal(t, "first", got[0].Description)
	// A nil down marks the migration irreversible; reverting it fails, so its version stays recorded.
	require.NotNil(t, got[2].Down)
	require.ErrorIs(t, got[2].Down(t.Context(), nil), ErrIrreversibleMigration)
	require.NoError(t, got[0].Down(t.Context(), nil), "a registered down runs as given")

	got[0].Description = "mutated"
	require.Equal(t, "first", Registered()[0].Description, "callers get a copy")
}

// TestRunWithoutMigrations is serial: it swaps the process-wide migration registry. With nothing registered no provider is built,
// so no database is needed.
func TestRunWithoutMigrations(t *testing.T) { //nolint:paralleltest // swaps the process-wide migration registry
	isolateRegistry(t)

	stub := &sql.DB{} // never used: nothing is registered
	require.NoError(t, Run(t.Context(), stub))
	require.NoError(t, Down(t.Context(), stub))
	status, err := Status(t.Context(), stub)
	require.NoError(t, err)
	require.Nil(t, status)

	require.Error(t, Run(t.Context(), nil), "a nil database is rejected")
}

func TestTableNameValidation(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", "select", "join.ledger", " spaced ", "a.b.c", "9table", "Mixed", "bad-name", strings.Repeat("s", 64) + ".ledger", "app." + strings.Repeat("t", 64)} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			stub := new(sql.DB)
			option := WithTableName(name)
			require.ErrorIs(t, Run(t.Context(), stub, option), ErrInvalidTableName)
			require.ErrorIs(t, Down(t.Context(), stub, option), ErrInvalidTableName)
			_, err := Status(t.Context(), stub, option)
			require.ErrorIs(t, err, ErrInvalidTableName)
		})
	}
}

func TestCheckoutDeadline(t *testing.T) { //nolint:paralleltest // replaces the process-wide registry
	isolateRegistry(t)
	MustRegister(1, "first", noop, nil)
	db, _ := testhelpers.NewFakeSQL(t, nil)
	db.SetMaxOpenConns(1)
	holder, err := db.Conn(t.Context())
	require.NoError(t, err)
	defer func() { _ = holder.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	err = Run(ctx, db, WithLockTimeout(10*time.Millisecond))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, ctx.Err(), "lock budget expires before caller context")
}

func TestLedgerSnapshot(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Columns: []string{"version_id", "is_applied", "tstamp"}, Rows: [][]driver.Value{{int64(1), true, nil}, {int64(2), true, nil}, {int64(2), false, nil}}}
	})
	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	rows, err := recordedVersions(t.Context(), conn, "ledger")
	require.NoError(t, err)
	require.True(t, rows[1].applied)
	require.True(t, rows[1].at.IsZero())
	require.False(t, rows[2].applied)
	require.Len(t, fake.Calls(), 1)
}

func TestSafeMigrationFailure(t *testing.T) {
	t.Parallel()
	for _, kind := range []error{ErrIrreversibleMigration, ErrInvalidMigration, ErrDirectConnectionRequired} {
		t.Run(kind.Error(), func(t *testing.T) {
			t.Parallel()
			err := diagnostic("up", &goose.PartialError{Failed: &goose.MigrationResult{Source: &goose.Source{Version: 17}}, Err: errors.Join(errors.New("password=secret"), kind, context.Canceled)})
			require.ErrorIs(t, err, kind)
			require.ErrorIs(t, err, context.Canceled)
			require.ErrorContains(t, err, "up version 17")
			require.NotContains(t, err.Error(), "secret")
		})
	}
}

type unavailable struct {
	driver.Connector
	cause error
}

func (u unavailable) Connect(context.Context) (driver.Conn, error) { return nil, u.cause }

func TestPublicDiagnostics(t *testing.T) { //nolint:paralleltest // swaps the process-wide registry
	isolateRegistry(t)
	MustRegister(1, "first", noop, nil)
	db := sql.OpenDB(unavailable{cause: &pgconn.PgError{Code: "08006", Message: "password=secret"}})
	t.Cleanup(func() { _ = db.Close() })
	for _, test := range []struct {
		name string
		call func(context.Context, *sql.DB, ...Option) error
	}{
		{"up", Run}, {"down", Down}, {"status", func(ctx context.Context, db *sql.DB, opts ...Option) error {
			_, err := Status(ctx, db, opts...)
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, test.call(t.Context(), db, WithTableName("bad-name")), ErrInvalidTableName)
			err := test.call(t.Context(), db)
			require.ErrorContains(t, err, test.name)
			require.ErrorContains(t, err, "SQLSTATE 08006")
			require.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestOptionFieldDocumentation(t *testing.T) {
	t.Parallel()
	source, err := parser.ParseFile(token.NewFileSet(), "options.go", nil, parser.ParseComments|parser.SkipObjectResolution)
	require.NoError(t, err)
	var opts *ast.StructType
	for node := range ast.Preorder(source) {
		if spec, ok := node.(*ast.TypeSpec); ok && spec.Name.Name == "options" {
			opts = spec.Type.(*ast.StructType)
			break
		}
	}
	require.NotNil(t, opts)
	require.NotEmpty(t, opts.Fields.List)
	for _, field := range opts.Fields.List {
		name := field.Names[0].Name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			want := "With" + strings.ToUpper(name[:1]) + name[1:] + " "
			require.True(t, strings.HasPrefix(field.Doc.Text(), want), "field comment must start with %q", want)
		})
	}
}

func TestStepPanicReleasesTransaction(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	step := transactionalStep(func(context.Context, *sql.Tx) error { panic("step panic") }, time.Second)
	require.PanicsWithValue(t, "step panic", func() { _ = step.RunTx(t.Context(), tx) })
	require.Equal(t, 1, fake.Rollbacks())
}

type blockedLock struct{ driver.Conn }

func (blockedLock) Close() error { return nil }
func (blockedLock) ExecContext(ctx context.Context, _ string, _ []driver.NamedValue) (driver.Result, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestLockQueryDeadline(t *testing.T) {
	t.Parallel()
	db := sql.OpenDB(&singleSession{conn: blockedLock{}})
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	acquire, stop := context.WithTimeout(ctx, 10*time.Millisecond)
	defer stop()
	err = withLock(ctx, acquire, conn, newOptions(), func(*sql.DB) error { t.Fatal("work ran without lock"); return nil })
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, ctx.Err(), "stalled query used the acquisition deadline")
}

func TestInjectedRegistryOperations(t *testing.T) {
	t.Parallel()
	var r Registry
	require.NoError(t, r.Register(2, "private", noop, nil))
	require.ErrorIs(t, r.Register(2, "duplicate", noop, nil), ErrInvalidMigration)
	// A canceled checkout distinguishes nonempty registries from the empty fast path.
	db, _ := testhelpers.NewFakeSQL(t, nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, tc := range []struct {
		name string
		call func(context.Context, *sql.DB, ...Option) error
	}{
		{"run", Run}, {"down", Down}, {"status", func(ctx context.Context, db *sql.DB, opts ...Option) error {
			_, err := Status(ctx, db, opts...)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, tc.call(ctx, db, WithRegistry(&r)), context.Canceled)
			require.NoError(t, tc.call(ctx, db, WithRegistry(new(Registry))))
		})
	}
}

func TestStepLockTimeout(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		timeout time.Duration
		want    []any
	}{
		{0, nil}, {-time.Second, nil}, {5 * time.Second, []any{"5000ms"}}, {time.Microsecond, []any{"1ms"}},
		{time.Duration(math.MaxInt64), []any{"2147483647ms"}},
	} {
		t.Run(tc.timeout.String(), func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, nil)
			tx, err := db.BeginTx(t.Context(), nil)
			require.NoError(t, err)
			var ran bool
			step := transactionalStep(func(ctx context.Context, tx *sql.Tx) error {
				ran = true
				_, err := tx.ExecContext(ctx, "CREATE TABLE items (id int)")
				return err
			}, tc.timeout)
			require.NoError(t, step.RunTx(t.Context(), tx))
			require.True(t, ran)
			calls := fake.Calls()
			if tc.want == nil {
				require.Len(t, calls, 1)
				return
			}
			require.Len(t, calls, 2)
			require.Contains(t, calls[0].Query, "set_config('lock_timeout', $1, true)")
			require.Equal(t, tc.want, calls[0].Args)
			require.True(t, calls[0].InTx, "the timeout is local to the migration transaction")
			require.Equal(t, "CREATE TABLE items (id int)", calls[1].Query)
		})
	}

	db, _ := testhelpers.NewFakeSQL(t, func(query string, _ []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Err: errors.New("refused")}
	})
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	step := transactionalStep(func(context.Context, *sql.Tx) error { t.Fatal("step ran without its lock timeout"); return nil }, time.Second)
	require.ErrorContains(t, step.RunTx(t.Context(), tx), "refused")
}

func TestCreateSchema(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		table  string
		exists bool
		want   []string
	}{
		{"ledger", false, nil},
		{"app.ledger", true, []string{"pg_namespace"}},
		{"app.ledger", false, []string{"pg_namespace", "CREATE SCHEMA app"}},
	} {
		t.Run(fmt.Sprint(tc.table, tc.exists), func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, func(query string, _ []any) testhelpers.FakeSQLReply {
				if strings.Contains(query, "pg_namespace") {
					return testhelpers.FakeSQLReply{Columns: []string{"exists"}, Rows: [][]driver.Value{{tc.exists}}}
				}
				return testhelpers.FakeSQLReply{}
			})
			require.NoError(t, createSchema(t.Context(), db, tc.table))
			calls := fake.Calls()
			require.Len(t, calls, len(tc.want))
			for i, want := range tc.want {
				require.Contains(t, calls[i].Query, want)
			}
			if len(calls) > 0 {
				require.Equal(t, []any{"app"}, calls[0].Args)
			}
		})
	}
}
