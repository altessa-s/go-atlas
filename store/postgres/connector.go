// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
)

const (
	statementTimeoutSetting = "statement_timeout"
	lockTimeoutSetting      = "lock_timeout"
	idleTimeoutSetting      = "idle_in_transaction_session_timeout"
)

var (
	_ driver.Connector   = (*connector)(nil)
	_ driver.ConnBeginTx = (*connection)(nil)
	_ driver.Tx          = (*driverTx)(nil)
)

// NewConnector creates pgx connections with the configured transaction and session timeouts.
// Use Open for a managed pool with startup checks.
func NewConnector(dsn redacted.RedactedString, opts ...Option) (driver.Connector, error) {
	return newConnector(dsn, newOptions(opts...))
}

func newConnector(dsn redacted.RedactedString, o *options) (driver.Connector, error) {
	if strings.TrimSpace(dsn.Expose()) == "" {
		return nil, errInvalidDSN
	}
	if o.connectTimeout < time.Millisecond {
		return nil, errors.New("connect timeout must be at least one millisecond")
	}
	cc, err := pgx.ParseConfig(dsn.Expose())
	if err != nil {
		return nil, errInvalidDSN
	}
	if o.pooling != PoolingDirect && o.pooling != PoolingTransaction {
		return nil, fmt.Errorf("unknown PostgreSQL pooling mode %q", o.pooling)
	}
	cc.ConnectTimeout = o.connectTimeout
	if o.tracer != nil {
		cc.Tracer = NewQueryTracer(o.tracer)
	}
	transactionPooling := o.pooling == PoolingTransaction
	var settings strings.Builder
	for _, setting := range []struct {
		name    string
		timeout time.Duration
	}{
		{statementTimeoutSetting, o.statementTimeout},
		{lockTimeoutSetting, o.lockTimeout},
		{idleTimeoutSetting, o.idleTransactionTimeout},
	} {
		ms := setting.timeout.Milliseconds()
		if ms < 1 || ms > math.MaxInt32 {
			return nil, fmt.Errorf("%s must be between 1 and %d milliseconds", setting.name, math.MaxInt32)
		}
		value := strconv.FormatInt(ms, 10)
		fmt.Fprintf(&settings, "; SET LOCAL %s = %s", setting.name, value)
		for key := range cc.RuntimeParams {
			if strings.EqualFold(key, setting.name) {
				delete(cc.RuntimeParams, key)
			}
		}
		if !transactionPooling {
			cc.RuntimeParams[setting.name] = value
		}
	}
	if transactionPooling {
		if cc.DefaultQueryExecMode == pgx.QueryExecModeSimpleProtocol || cc.DefaultQueryExecMode == pgx.QueryExecModeDescribeExec {
			return nil, fmt.Errorf("query execution mode %s is incompatible with transaction pooling", cc.DefaultQueryExecMode)
		}
		for key := range cc.RuntimeParams {
			switch strings.ToLower(key) {
			case "application_name", "client_encoding", "datestyle", "timezone", "standard_conforming_strings":
			default:
				return nil, errors.New("unsupported startup parameter for transaction pooling")
			}
		}
	}
	return &connector{
		Connector: stdlib.GetConnector(*cc), settings: settings.String(), retireOnPrepareError: transactionPooling,
	}, nil
}

type connector struct {
	driver.Connector
	settings             string
	retireOnPrepareError bool
}

func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	raw, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, diagnostic("connect PostgreSQL", err)
	}
	conn, ok := raw.(*stdlib.Conn)
	if !ok {
		return nil, errors.Join(fmt.Errorf("unexpected pgx driver connection %T", raw), raw.Close())
	}
	return &connection{stdlibConn: conn, settings: c.settings, retireOnPrepareError: c.retireOnPrepareError}, nil
}

type stdlibConn = stdlib.Conn

// connection retires transaction-pooled sessions before reuse after a failed prepare.
// PgBouncer can retain the failed statement name and reject a new Parse with the same name.
type connection struct {
	*stdlibConn
	settings             string
	retireOnPrepareError bool
	retire               bool
}

func (c *connection) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *connection) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	conn := c.Conn()
	if conn.IsClosed() {
		return nil, driver.ErrBadConn
	}
	options, err := beginOptions(ctx, opts, c.settings)
	if err != nil {
		return nil, err
	}
	tx, err := conn.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return driverTx{ctx: ctx, tx: tx}, nil
}

func (c *connection) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := c.stdlibConn.QueryContext(ctx, query, args)
	return rows, c.statementError(ctx, err)
}

func (c *connection) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	result, err := c.stdlibConn.ExecContext(ctx, query, args)
	return result, c.statementError(ctx, err)
}

func (c *connection) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

func (c *connection) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	stmt, err := c.stdlibConn.PrepareContext(ctx, query)
	return stmt, c.statementError(ctx, err)
}

func (c *connection) ResetSession(ctx context.Context) error {
	if c.retire {
		return driver.ErrBadConn
	}
	return c.stdlibConn.ResetSession(ctx)
}

func (c *connection) statementError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if sqldialect.SafeToRetry(err) {
		// An exhausted retry budget suppresses ErrBadConn. Still retire this socket.
		c.retire = true
		if ctx.Err() != nil {
			return ctx.Err()
		}
	} else if c.retireOnPrepareError {
		if _, failed := errors.AsType[*pgconn.PrepareError](err); failed {
			c.retire = true
		}
	}
	if errors.Is(err, driver.ErrBadConn) {
		c.retire = true
	}
	return sqldialect.DriverError(ctx, err)
}

func beginOptions(ctx context.Context, opts driver.TxOptions, settings string) (pgx.TxOptions, error) {
	var tx pgx.TxOptions
	switch sql.IsolationLevel(opts.Isolation) {
	case sql.LevelDefault:
	case sql.LevelReadUncommitted:
		tx.IsoLevel = pgx.ReadUncommitted
	case sql.LevelReadCommitted:
		tx.IsoLevel = pgx.ReadCommitted
	case sql.LevelRepeatableRead, sql.LevelSnapshot:
		tx.IsoLevel = pgx.RepeatableRead
	case sql.LevelSerializable:
		tx.IsoLevel = pgx.Serializable
	default:
		return tx, fmt.Errorf("unsupported isolation: %v", opts.Isolation)
	}
	begin := "BEGIN"
	if tx.IsoLevel != "" {
		begin += " ISOLATION LEVEL " + string(tx.IsoLevel)
	}
	if opts.ReadOnly {
		tx.AccessMode = pgx.ReadOnly
		begin += " " + string(tx.AccessMode)
	}
	tx.BeginQuery = begin + settings + transactionSettingsSQL(ctx)
	return tx, nil
}

type driverTx struct {
	ctx context.Context
	tx  pgx.Tx
}

func (t driverTx) Commit() error   { return t.tx.Commit(t.ctx) }
func (t driverTx) Rollback() error { return t.tx.Rollback(t.ctx) }
