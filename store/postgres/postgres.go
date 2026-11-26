// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"cmp"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"runtime"
	"slices"
	"sync"
	"time"
	"weak"

	"github.com/altessa-s/go-slogx"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

const (
	identityResolveTimeout = 5 * time.Second
	controlIdentityQuery   = "SELECT system_identifier::text, current_database() FROM pg_control_system()"
	databaseIdentityQuery  = "SELECT current_database()"
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
	registerPoolLogger(db, o.logger)
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

// Verify eagerly resolves the database identity of every pool in the group and
// returns [ErrDatabaseMismatch] if any two pools serve different databases.
func (p Pools) Verify(ctx context.Context) error {
	var groupID dbIdentity
	for _, name := range slices.Sorted(maps.Keys(p)) {
		db := p[name]
		if db == nil {
			return coreerrs.WrapOperation(coreerrs.ErrInvalidArgument, "nil PostgreSQL pool")
		}
		id, err := resolvePoolIdentity(ctx, db)
		if err != nil {
			return err
		}
		if groupID.database == "" {
			groupID = id
			continue
		}
		if !groupID.equal(id) {
			return fmt.Errorf("%w: pool %q serves another database", ErrDatabaseMismatch, name)
		}
		groupID.systemID = cmp.Or(groupID.systemID, id.systemID)
	}
	return nil
}

// Binding returns the named pool and a resolver for transactions owned by the group.
// Pools in the group must serve the same logical database; if ctx carries a transaction
// from another pool in the group whose resolved database identity differs from the bound
// pool, the returned transaction handle rejects operations with [ErrDatabaseMismatch]
// instead of running outside the transaction. A missing name returns a nil database,
// which the caller must check.
func (p Pools) Binding(name string) (*sql.DB, func(context.Context) (*sql.Tx, bool)) {
	boundPool := p[name]
	members := make(map[*sql.DB]struct{}, len(p))
	for _, db := range p {
		members[db] = struct{}{}
	}
	return boundPool, func(ctx context.Context) (*sql.Tx, bool) {
		t, ok := ctx.Value(transactionKey{}).(*transaction)
		if !ok {
			return nil, false
		}
		if _, member := members[t.db]; !member {
			return nil, false
		}
		if boundPool == nil {
			return failingTx(ctx, ErrDatabaseMismatch), true
		}
		if t.db == boundPool {
			return t.tx, t.open()
		}
		if !t.open() {
			return t.tx, false
		}
		boundID, err := resolvePoolIdentity(ctx, boundPool)
		if err != nil {
			return failingTx(ctx, err), true
		}
		txID, err := resolvePoolIdentity(ctx, t.db)
		if err != nil {
			return failingTx(ctx, err), true
		}
		if !boundID.equal(txID) {
			return failingTx(ctx, ErrDatabaseMismatch), true
		}
		return t.tx, t.open()
	}
}

type dbIdentity struct {
	systemID string
	database string
}

func (a dbIdentity) equal(b dbIdentity) bool {
	if a.database == "" || b.database == "" || a.database != b.database {
		return false
	}
	return a.systemID == "" || b.systemID == "" || a.systemID == b.systemID
}

type poolIdentityEntry struct {
	mu       sync.Mutex
	resolved bool
	id       dbIdentity
	logger   *slog.Logger
	infoOnce sync.Once
}

var poolIdentities sync.Map // map[weak.Pointer[sql.DB]]*poolIdentityEntry

func identityEntryFor(db *sql.DB) *poolIdentityEntry {
	key := weak.Make(db)
	if existing, ok := poolIdentities.Load(key); ok {
		if entry, ok := existing.(*poolIdentityEntry); ok {
			return entry
		}
	}
	fresh := &poolIdentityEntry{}
	actual, loaded := poolIdentities.LoadOrStore(key, fresh)
	if !loaded {
		runtime.AddCleanup(db, func(k weak.Pointer[sql.DB]) {
			poolIdentities.Delete(k)
		}, key)
	}
	entry, ok := actual.(*poolIdentityEntry)
	if !ok {
		return fresh
	}
	return entry
}

func registerPoolLogger(db *sql.DB, logger *slog.Logger) {
	if db == nil || logger == nil {
		return
	}
	entry := identityEntryFor(db)
	entry.mu.Lock()
	entry.logger = logger
	entry.mu.Unlock()
}

func resolvePoolIdentity(ctx context.Context, db *sql.DB) (dbIdentity, error) {
	entry := identityEntryFor(db)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.resolved {
		return entry.id, nil
	}

	queryCtx, cancel := corecontext.WithMaxTimeout(corecontext.OrBackground(ctx), identityResolveTimeout)
	defer cancel()

	var id dbIdentity
	err := db.QueryRowContext(queryCtx, controlIdentityQuery).Scan(&id.systemID, &id.database)
	if err != nil {
		if !isUnreadableControlSystem(err) {
			return dbIdentity{}, diagnostic("verify PostgreSQL database identity", err)
		}
		if fallbackErr := db.QueryRowContext(queryCtx, databaseIdentityQuery).Scan(&id.database); fallbackErr != nil {
			return dbIdentity{}, diagnostic("verify PostgreSQL database identity", fallbackErr)
		}
		logger := cmp.Or(entry.logger, slog.Default())
		entry.infoOnce.Do(func() {
			logger.InfoContext(queryCtx, "PostgreSQL cluster identity could not be verified; using database name only", slogx.Module("postgres"))
		})
	}
	if id.database == "" {
		return dbIdentity{}, errors.New("empty PostgreSQL database identity")
	}
	entry.id = id
	entry.resolved = true
	return id, nil
}

func isUnreadableControlSystem(err error) bool {
	switch sqldialect.SQLState(err) {
	case "42501", "42883", "0A000":
		return true
	default:
		return false
	}
}

func failingTx(ctx context.Context, err error) *sql.Tx {
	db := sql.OpenDB(failingTxConnector{err: err})
	tx, _ := db.BeginTx(context.WithoutCancel(ctx), nil)
	_ = db.Close()
	return tx
}

type failingTxConnector struct{ err error }

func (c failingTxConnector) Connect(context.Context) (driver.Conn, error) {
	return failingTxConn(c), nil
}
func (c failingTxConnector) Driver() driver.Driver            { return c }
func (c failingTxConnector) Open(string) (driver.Conn, error) { return failingTxConn(c), nil }

type failingTxConn struct{ err error }

func (c failingTxConn) Prepare(string) (driver.Stmt, error) { return nil, c.err }
func (c failingTxConn) PrepareContext(context.Context, string) (driver.Stmt, error) {
	return nil, c.err
}
func (failingTxConn) Close() error                { return nil }
func (c failingTxConn) Begin() (driver.Tx, error) { return failingDriverTx(c), nil }
func (c failingTxConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return failingDriverTx(c), nil
}
func (c failingTxConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return nil, c.err
}
func (c failingTxConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return nil, c.err
}

type failingDriverTx struct{ err error }

func (t failingDriverTx) Commit() error { return t.err }
func (failingDriverTx) Rollback() error { return nil }
