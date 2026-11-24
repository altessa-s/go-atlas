// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"cmp"
	"context"
	"crypto/tls"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestBeginQuery(t *testing.T) {
	t.Parallel()
	conn, err := NewConnector("postgres://localhost/db", WithStatementTimeout(time.Second), WithLockTimeout(50*time.Millisecond), WithIdleTransactionTimeout(9*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	const settings = "; SET LOCAL statement_timeout = 1000; SET LOCAL lock_timeout = 50; SET LOCAL idle_in_transaction_session_timeout = 9000"
	for _, test := range []struct {
		level  sql.IsolationLevel
		clause string
	}{
		{sql.LevelDefault, ""}, {sql.LevelReadUncommitted, "read uncommitted"},
		{sql.LevelReadCommitted, "read committed"}, {sql.LevelRepeatableRead, "repeatable read"},
		{sql.LevelSnapshot, "repeatable read"}, {sql.LevelSerializable, "serializable"},
		{sql.LevelWriteCommitted, "unsupported"}, {sql.LevelLinearizable, "unsupported"}, {99, "unsupported"},
	} {
		for _, readOnly := range []bool{false, true} {
			opts, err := beginOptions(t.Context(), driver.TxOptions{Isolation: driver.IsolationLevel(test.level), ReadOnly: readOnly}, conn.(*connector).settings)
			if test.clause == "unsupported" {
				if err == nil {
					t.Errorf("%s readOnly=%t: expected unsupported isolation", test.level, readOnly)
				}
				continue
			}
			want := "BEGIN"
			if test.clause != "" {
				want += " ISOLATION LEVEL " + test.clause
			}
			access := ""
			if readOnly {
				access = "read only"
				want += " " + access
			}
			if err != nil || opts.BeginQuery != want+settings || string(opts.IsoLevel) != test.clause || string(opts.AccessMode) != access {
				t.Errorf("%s readOnly=%t: got %+v, %v; want %q", test.level, readOnly, opts, err, want+settings)
			}
		}
	}
}

func TestConnectionDriverInterfaces(t *testing.T) {
	t.Parallel()
	wrapped, base := reflect.TypeFor[*connection](), reflect.TypeFor[*stdlib.Conn]()
	for _, iface := range []reflect.Type{
		reflect.TypeFor[driver.Conn](), reflect.TypeFor[driver.ConnBeginTx](), reflect.TypeFor[driver.ConnPrepareContext](),
		reflect.TypeFor[driver.Pinger](), reflect.TypeFor[driver.Execer](), reflect.TypeFor[driver.ExecerContext](),
		reflect.TypeFor[driver.Queryer](), reflect.TypeFor[driver.QueryerContext](), reflect.TypeFor[driver.NamedValueChecker](),
		reflect.TypeFor[driver.SessionResetter](), reflect.TypeFor[driver.Validator](),
	} {
		if wrapped.Implements(iface) != base.Implements(iface) {
			t.Errorf("%v: stdlib=%t wrapper=%t", iface, base.Implements(iface), wrapped.Implements(iface))
		}
	}
}

func TestConnectionResetAfterStatementError(t *testing.T) {
	t.Parallel()
	for _, mode := range []PoolingMode{PoolingDirect, PoolingTransaction} {
		for _, operation := range []string{"query", "exec", "prepare", "legacy prepare", "simple exec"} {
			t.Run(string(mode)+"/"+operation, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				conn := statementErrorConnection(ctx, t, mode)
				const query = "SELECT $1::integer"
				args := []driver.NamedValue{{Ordinal: 1, Value: int64(1)}}
				var err error
				switch operation {
				case "query":
					_, err = conn.QueryContext(ctx, query, args)
				case "exec":
					_, err = conn.ExecContext(ctx, query, args)
				case "prepare":
					_, err = conn.PrepareContext(ctx, query)
				case "legacy prepare":
					_, err = conn.Prepare(query)
				case "simple exec":
					_, err = conn.ExecContext(ctx, "SELECT unavailable", nil)
				}
				var prepareErr *pgconn.PrepareError
				var pgErr *pgconn.PgError
				wantPrepare := operation != "simple exec"
				if !errors.As(err, &pgErr) || pgErr.Code != "42P01" || errors.As(err, &prepareErr) != wantPrepare {
					t.Fatalf("statement error = %v, want SQLSTATE 42P01 (prepare error: %t)", err, wantPrepare)
				}
				if conn.Conn().IsClosed() {
					t.Fatal("statement error closed the connection")
				}
				var want error
				if mode == PoolingTransaction && wantPrepare {
					want = driver.ErrBadConn
				}
				if got := conn.ResetSession(ctx); got != want {
					t.Fatalf("ResetSession = %v, want %v", got, want)
				}
			})
		}
	}
}

func TestTransactionSettings(t *testing.T) {
	t.Parallel()
	for _, budget := range []time.Duration{-time.Second, time.Nanosecond, 1<<63 - 1} {
		if _, err := WithTransactionSettings(t.Context(), nil, TransactionSettings{StatementTimeout: budget}); err == nil {
			t.Errorf("accepted %v", budget)
		}
	}
	ctx, err := WithTransactionSettings(t.Context(), nil, TransactionSettings{ForceCustomPlan: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err = WithTransactionSettings(ctx, nil, TransactionSettings{StatementTimeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	opts, err := beginOptions(ctx, driver.TxOptions{}, "")
	if err != nil || !strings.HasSuffix(opts.BeginQuery, "; SET LOCAL plan_cache_mode = force_custom_plan; SET LOCAL statement_timeout = '3000ms'") {
		t.Fatalf("%s: %v", opts.BeginQuery, err)
	}
}

func statementErrorConnection(ctx context.Context, t *testing.T, mode PoolingMode) *connection {
	t.Helper()
	const dsn = "postgres://test@127.0.0.1/db?sslmode=disable"
	outer, err := NewConnector(dsn, WithPooling(mode))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	client, server := net.Pipe()
	done := make(chan error, 1)
	go func() {
		defer server.Close()
		done <- serveStatementError(pgproto3.NewBackend(server, server))
	}()
	t.Cleanup(func() {
		client.Close()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	cfg.DialFunc = func(context.Context, string, string) (net.Conn, error) { return client, nil }
	wrapped := outer.(*connector)
	wrapped.Connector = stdlib.GetConnector(*cfg, stdlib.OptionShouldPing(func(context.Context, stdlib.ShouldPingParams) bool { return false }))
	raw, err := wrapped.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := raw.Close(); err != nil {
			t.Error(err)
		}
	})
	return raw.(*connection)
}

func serveStatementError(backend *pgproto3.Backend) error {
	if _, err := backend.ReceiveStartupMessage(); err != nil {
		return err
	}
	backend.Send(&pgproto3.AuthenticationOk{})
	backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
	if err := backend.Flush(); err != nil {
		return err
	}
	for {
		msg, err := backend.Receive()
		if err != nil {
			return err
		}
		if query, ok := msg.(*pgproto3.Query); ok {
			status := byte('I')
			switch query.String {
			case "BEGIN; SET LOCAL statement_timeout = 10000; SET LOCAL lock_timeout = 3000; SET LOCAL idle_in_transaction_session_timeout = 30000":
				status = 'T'
			case "rollback":
			default:
				status = 0
			}
			if status != 0 {
				backend.Send(&pgproto3.CommandComplete{CommandTag: []byte("OK")})
				backend.Send(&pgproto3.ReadyForQuery{TxStatus: status})
				if err := backend.Flush(); err != nil {
					return err
				}
				continue
			}
		}
		switch msg.(type) {
		case *pgproto3.Query, *pgproto3.Sync:
			backend.Send(&pgproto3.ErrorResponse{Severity: "ERROR", Code: "42P01", Message: "undefined table"})
			backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
			if err := backend.Flush(); err != nil {
				return err
			}
		case *pgproto3.Terminate:
			return nil
		}
	}
}

func TestLegacyBeginAppliesTransactionSettings(t *testing.T) {
	conn := statementErrorConnection(t.Context(), t, PoolingTransaction)
	// The protocol fixture accepts only BEGIN with the configured local settings.
	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
}

func TestMixedStatementFailureSuppressesDriverRetry(t *testing.T) {
	c := new(connection)
	err := c.statementError(t.Context(), errors.Join(driver.ErrBadConn, errors.New("lost response")))
	if err == nil || errors.Is(err, driver.ErrBadConn) {
		t.Fatalf("ambiguous failure permits database/sql replay: %v", err)
	}
}

func TestTLSEndpoints(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"allow", "prefer", "disable", "require"} {
		for _, name := range []string{"", "service.internal"} {
			t.Run(mode+"/"+name, func(t *testing.T) {
				t.Parallel()
				cc, err := pgx.ParseConfig("host=first,second,second port=5432,5433,5434 sslmode=" + mode)
				require.NoError(t, err)
				cfg := &tls.Config{RootCAs: testhelpers.NewCA(t).Pool, ServerName: name, MinVersion: tls.VersionTLS13}
				applyTLS(cc, cfg)
				require.Len(t, cc.Fallbacks, 2)
				require.Equal(t, name, cfg.ServerName)
				check := func(host string, target *tls.Config) {
					t.Helper()
					require.NotNil(t, target)
					require.NotSame(t, cfg, target)
					require.Equal(t, cmp.Or(name, host), target.ServerName)
					require.False(t, target.InsecureSkipVerify)
					require.Same(t, cfg.RootCAs, target.RootCAs)
				}
				check("first", cc.TLSConfig)
				for i, f := range cc.Fallbacks {
					require.Equal(t, "second", f.Host)
					require.Equal(t, []uint16{5433, 5434}[i], f.Port)
					check("second", f.TLSConfig)
					require.NotSame(t, cc.TLSConfig, f.TLSConfig)
				}
				require.NotSame(t, cc.Fallbacks[0].TLSConfig, cc.Fallbacks[1].TLSConfig)
			})
		}
	}
}

func TestInsecureTLSRequiresOptIn(t *testing.T) {
	t.Parallel()
	config := WithTLSConfig(&tls.Config{InsecureSkipVerify: true}) //nolint:gosec // exercise the explicit opt-in
	_, err := NewConnector("host=db", config)
	require.Error(t, err)
	_, err = NewConnector("host=db", config, WithAllowInsecureSkipVerify())
	require.NoError(t, err)
}
