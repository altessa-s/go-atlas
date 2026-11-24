// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgrestest

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type rowFunc func(...any) error

func (f rowFunc) Scan(dest ...any) error { return f(dest...) }

type catalog struct {
	locked, created, marked, dropped bool
	collision, markFails             bool
	current                          database
	idle                             bool
}

func (c *catalog) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	return rowFunc(func(dest ...any) error {
		switch {
		case strings.Contains(query, "pg_try_advisory_lock"):
			c.locked = true
			*dest[0].(*bool) = true
		case strings.HasPrefix(query, "SELECT oid"):
			*dest[0].(*uint32) = c.current.oid
		default:
			*dest[0].(*bool) = c.locked && args[0] == c.current.name && args[1] == c.current.oid && args[2] == c.current.marker
			*dest[1].(*bool) = c.idle
		}
		return nil
	})
}
func (c *catalog) Exec(_ context.Context, query string, _ ...any) (pgconn.CommandTag, error) {
	switch {
	case !c.locked:
		return pgconn.CommandTag{}, errors.New("DDL without lock")
	case strings.HasPrefix(query, "CREATE"):
		if c.collision {
			return pgconn.CommandTag{}, errors.New("collision")
		}
		c.created = true
	case strings.HasPrefix(query, "COMMENT"):
		if c.markFails {
			return pgconn.CommandTag{}, errors.New("mark failed")
		}
		c.marked = true
	case strings.HasPrefix(query, "DROP"):
		c.dropped = true
	}
	return pgconn.CommandTag{}, nil
}

func TestOwnership(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                                        string
		collision, markFails, replace, remark, busy bool
	}{
		{name: "owned"}, {name: "create collision", collision: true}, {name: "unconfirmed marker", markFails: true}, {name: "same name and marker, new OID", replace: true}, {name: "changed marker", remark: true}, {name: "active session", busy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := &catalog{current: database{name: "it_test", oid: 7}, idle: !tc.busy, collision: tc.collision, markFails: tc.markFails}
			d, err := createOwned(t.Context(), c, c.current.name)
			if tc.collision || tc.markFails {
				require.Error(t, err)
				require.Zero(t, d)
				require.Equal(t, !tc.collision, c.created)
				require.False(t, c.marked)
				require.False(t, c.dropped)
				return
			}
			require.NoError(t, err)
			require.True(t, c.marked)
			c.current.marker = d.marker
			if tc.replace {
				c.current.oid++
			}
			if tc.remark {
				c.current.marker = ""
			}
			err = dropChecked(t.Context(), c, d)
			if tc.replace || tc.remark || tc.busy {
				require.Error(t, err)
				require.False(t, c.dropped)
			} else {
				require.NoError(t, err)
				require.True(t, c.dropped)
			}
		})
	}
}
func TestIsMarker(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		Marker:                                  true,
		Marker + "; instance 0123456789abcdef":  true,
		Marker + "; kept for diagnostics":       false,
		Marker + "; instance 0123456789abcde":   false,
		Marker + "; instance 0123456789ABCDEF":  false,
		Marker + "; instance 0123456789abcdef0": false,
		"x" + Marker:                            false,
		"":                                      false,
	}
	for comment, want := range tests {
		t.Run(comment, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, want, isMarker(comment))
		})
	}
}

type permissiveError struct{}

func (permissiveError) Error() string { return "driver-secret" }
func (permissiveError) Is(error) bool { return true }

type diagnosticTB struct {
	testing.TB
	output string
}

func (tb *diagnosticTB) Fatalf(format string, args ...any) {
	tb.output = fmt.Sprintf(format, args...)
	panic("fatal")
}
func TestDiagnosticIgnoresPermissiveIs(t *testing.T) {
	t.Parallel()
	tb := &diagnosticTB{TB: t}
	require.PanicsWithValue(t, "fatal", func() { check(tb, "mark database", permissiveError{}) })
	require.True(t, strings.Contains(tb.output, "mark database failed"))
	require.False(t, strings.Contains(tb.output, "driver-secret"), "driver text in diagnostic")
}

// Model pgx's reset ping on the connection left idle by postgres.Open.
type checkoutContext struct {
	context.Context
	resetting bool
}

func (c *checkoutContext) Done() <-chan struct{} {
	if c.resetting {
		panic("interrupted during checkout")
	}
	return c.Context.Done()
}

type checkoutConnector struct {
	driver.Connector
	conn *checkoutConn
}

func (c checkoutConnector) Connect(context.Context) (driver.Conn, error) { return c.conn, nil }

type driverConn = driver.Conn

type checkoutConn struct {
	driverConn
	caller *checkoutContext
	closed bool
}

func (c *checkoutConn) ResetSession(ctx context.Context) error {
	c.caller.resetting = true
	defer func() { c.caller.resetting = false }()
	_ = ctx.Done()
	return nil
}

func (c *checkoutConn) Conn() *pgx.Conn { panic("interrupted identity check") }
func (c *checkoutConn) Close() error {
	c.closed = true
	return nil
}

func TestWorkingCheckPanicClosesSessionBeforeDrop(t *testing.T) {
	t.Parallel()
	ctx := &checkoutContext{Context: t.Context()}
	session := &checkoutConn{caller: ctx}
	db := sql.OpenDB(checkoutConnector{conn: session})
	boundedCleanup(t, func(ctx context.Context) {
		require.NoError(t, ctx.Err())
		require.True(t, session.closed, "target backend still connected at drop")
	})
	cleanup(t, "close working pool", func(ctx context.Context) error { return closePool(ctx, db) })
	require.NoError(t, db.PingContext(ctx))
	require.PanicsWithValue(t, "interrupted identity check", func() { _ = checkWorking(ctx, db, "target") })
	require.Zero(t, db.Stats().InUse)
}

func TestDropDiagnosticWithJoinedCauses(t *testing.T) {
	t.Parallel()
	cause := errors.Join(&pgconn.PgError{Code: "private", Message: "secret"},
		&pgconn.PgError{Code: "55006", Detail: "secret"}, &pgconn.PgError{Code: "42501"})
	err := dropDiagnostic("execute DROP DATABASE", fmt.Errorf("secret: %w", cause))
	require.EqualError(t, err, "execute DROP DATABASE failed (*fmt.wrapError, SQLSTATE 55006, 42501)")
}
