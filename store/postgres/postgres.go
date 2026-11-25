// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/altessa-s/go-slogx"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// Open creates a bounded pool and verifies connectivity and transaction-pool role defaults.
func Open(ctx context.Context, dsn redacted.RedactedString, opts ...Option) (pool *sql.DB, openErr error) {
	o := newOptions(opts...)
	if o.maxOpenConnections < 1 || o.maxIdleConnections < 0 || o.connectionMaxLifetime <= 0 || o.connectionMaxIdleTime <= 0 {
		return nil, errors.New("invalid PostgreSQL pool limits")
	}
	c, err := newConnector(dsn, o)
	if err != nil {
		return nil, err
	}
	owner := &poolConnector{Connector: c}
	db := sql.OpenDB(owner)
	defer func() {
		if pool == nil {
			openErr = errors.Join(openErr, diagnostic("close PostgreSQL", db.Close()))
		}
	}()
	db.SetMaxOpenConns(o.maxOpenConnections)
	db.SetMaxIdleConns(o.maxIdleConnections)
	db.SetConnMaxLifetime(o.connectionMaxLifetime)
	db.SetConnMaxIdleTime(o.connectionMaxIdleTime)

	if o.pooling == PoolingTransaction {
		err = checkPooledDefaults(ctx, db)
	} else {
		err = diagnostic("ping PostgreSQL", db.PingContext(ctx))
	}
	if err != nil {
		return nil, err
	}

	if o.meter != nil && !metrics.IsNoop(o.meter) {
		owner.stop = observePool(db, o.meter, o.poolName)
	}
	o.logger.DebugContext(ctx, "PostgreSQL pool opened", slogx.Module("postgres"), "pool", o.poolName)
	return db, nil
}

// Checker reports the pool as serving while the database answers a ping.
func Checker(db *sql.DB) health.Checker {
	return health.Func(func(ctx context.Context) health.ServingStatus {
		if err := db.PingContext(ctx); err != nil {
			return health.StatusNotServing
		}

		return health.StatusServing
	})
}

func checkPooledDefaults(ctx context.Context, db *sql.DB) error {
	var statement, lock, idle string
	err := db.QueryRowContext(ctx, `SELECT current_setting('statement_timeout'), current_setting('lock_timeout'),
		current_setting('idle_in_transaction_session_timeout')`).Scan(&statement, &lock, &idle)
	if err != nil {
		return diagnostic("read PostgreSQL role defaults", err)
	}
	for name, value := range map[string]string{
		statementTimeoutSetting: statement, lockTimeoutSetting: lock, idleTimeoutSetting: idle,
	} {
		if value == "0" {
			return fmt.Errorf("%w: ALTER ROLE <role> SET %s to a positive duration", ErrPooledTimeoutDefaults, name)
		}
	}
	return nil
}

// Pools contains independently sized database/sql pools keyed by configured name.
type Pools map[string]*sql.DB

// Close closes all pools and waits for their metric observers to stop.
func (p Pools) Close() error {
	var errs []error
	for _, db := range p {
		if err := db.Close(); err != nil {
			errs = append(errs, diagnostic("close PostgreSQL", err))
		}
	}
	return errors.Join(errs...)
}

// CloseContext starts closing every pool and waits until they close or ctx ends.
// Pool closure continues in the background if the context ends first.
func (p Pools) CloseContext(ctx context.Context) error {
	// database/sql has no context-aware Close. Start every close before waiting
	// so one stalled pool cannot prevent the others from closing.
	results := make(chan error, len(p))
	for _, db := range p {
		go func() { results <- diagnostic("close PostgreSQL", db.Close()) }()
	}
	var err error
	for range p {
		select {
		case closeErr := <-results:
			err = errors.Join(err, closeErr)
		case <-ctx.Done():
			// Include close errors already available when the wait expires.
			for len(results) > 0 {
				err = errors.Join(err, <-results)
			}
			return errors.Join(err, ctx.Err())
		}
	}
	return err
}

// Binding returns the named pool and a resolver for transactions owned by the group. Pools in the group must serve the same logical database.
// A missing name returns a nil database, which the caller must check.
func (p Pools) Binding(name string) (*sql.DB, func(context.Context) (*sql.Tx, bool)) {
	members := make(map[*sql.DB]struct{}, len(p))
	for _, db := range p {
		members[db] = struct{}{}
	}
	return p[name], func(ctx context.Context) (*sql.Tx, bool) {
		t, ok := ctx.Value(transactionKey{}).(*transaction)
		if !ok {
			return nil, false
		}
		if _, member := members[t.db]; !member {
			return nil, false
		}
		return t.tx, t.open()
	}
}
