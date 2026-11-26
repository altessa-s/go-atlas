// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
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
	identityFailureTTL     = time.Second
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

// Binding prepares the named pool and a resolver for transactions owned by the group.
// For groups with multiple distinct pools, call Binding before opening transactions.
// It resolves pool identities so that a resolver call never needs another connection.
// A transaction from a different database in the group is rejected with [ErrDatabaseMismatch].
// A missing name or failed identity lookup returns a construction error.
func (p Pools) Binding(ctx context.Context, name string) (*sql.DB, func(context.Context) (*sql.Tx, bool, error), error) {
	boundPool := p[name]
	if boundPool == nil {
		return nil, nil, coreerrs.WrapOperation(coreerrs.ErrInvalidArgument, "missing PostgreSQL pool")
	}
	identities := make(map[*sql.DB]dbIdentity, len(p))
	for _, db := range p {
		if db == nil {
			return nil, nil, coreerrs.WrapOperation(coreerrs.ErrInvalidArgument, "nil PostgreSQL pool")
		}
		identities[db] = dbIdentity{}
	}
	if len(identities) > 1 {
		for _, name := range slices.Sorted(maps.Keys(p)) {
			db := p[name]
			if identities[db].database != "" {
				continue
			}
			id, err := resolvePoolIdentity(ctx, db)
			if err != nil {
				return nil, nil, err
			}
			identities[db] = id
		}
	}
	boundID := identities[boundPool]
	return boundPool, func(ctx context.Context) (*sql.Tx, bool, error) {
		t := attached(ctx)
		if t == nil {
			return nil, false, nil
		}
		txID, member := identities[t.db]
		if !member {
			return nil, false, nil
		}
		open := t.open()
		if open && t.db != boundPool && !boundID.equal(txID) {
			return nil, false, ErrDatabaseMismatch
		}
		return t.tx, open, nil
	}, nil
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
	lookup   atomic.Pointer[poolIdentityLookup]
	logger   atomic.Pointer[slog.Logger]
	infoOnce sync.Once
}

// Read the result only after done is closed. A failed lookup can be replaced
// once its retry time passes.
type poolIdentityLookup struct {
	done    chan struct{}
	id      dbIdentity
	err     error
	retryAt time.Time
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
	identityEntryFor(db).logger.Store(logger)
}

func resolvePoolIdentity(ctx context.Context, db *sql.DB) (dbIdentity, error) {
	queryCtx, cancel := corecontext.WithMaxTimeout(corecontext.OrBackground(ctx), identityResolveTimeout)
	defer cancel()

	entry := identityEntryFor(db)
	for {
		if err := queryCtx.Err(); err != nil {
			return dbIdentity{}, err
		}
		current := entry.lookup.Load()
		if current != nil {
			select {
			case <-queryCtx.Done():
				return dbIdentity{}, queryCtx.Err()
			case <-current.done:
			}
			if current.err == nil || time.Now().Before(current.retryAt) {
				return current.id, current.err
			}
		}
		next := &poolIdentityLookup{done: make(chan struct{})}
		if !entry.lookup.CompareAndSwap(current, next) {
			continue
		}
		next.id, next.err = readPoolIdentity(queryCtx, db, entry)
		if next.err != nil {
			next.retryAt = time.Now().Add(identityFailureTTL)
		}
		close(next.done)
		return next.id, next.err
	}
}

func readPoolIdentity(queryCtx context.Context, db *sql.DB, entry *poolIdentityEntry) (dbIdentity, error) {
	var id dbIdentity
	err := db.QueryRowContext(queryCtx, controlIdentityQuery).Scan(&id.systemID, &id.database)
	if err != nil {
		if !isUnreadableControlSystem(err) {
			return dbIdentity{}, diagnostic("verify PostgreSQL database identity", err)
		}
		if fallbackErr := db.QueryRowContext(queryCtx, databaseIdentityQuery).Scan(&id.database); fallbackErr != nil {
			return dbIdentity{}, diagnostic("verify PostgreSQL database identity", fallbackErr)
		}
		logger := cmp.Or(entry.logger.Load(), slog.Default())
		entry.infoOnce.Do(func() {
			logger.InfoContext(queryCtx, "PostgreSQL cluster identity could not be verified; using database name only", slogx.Module("postgres"))
		})
	}
	if id.database == "" {
		return dbIdentity{}, errors.New("empty PostgreSQL database identity")
	}
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
