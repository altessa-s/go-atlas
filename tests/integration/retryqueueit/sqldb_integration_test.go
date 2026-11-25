// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retryqueueit_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/retryqueue"
	"github.com/altessa-s/go-atlas/reliability/retryqueue/storages/sqldb"
	"github.com/altessa-s/go-atlas/reliability/retryqueue/storages/storagetest"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	_ "github.com/jackc/pgx/v5/stdlib"

	retryqueueconfig "github.com/altessa-s/go-atlas/config/retryqueue"
	retryqueuefactory "github.com/altessa-s/go-atlas/factory/reliability/retryqueue"
)

// sqlTarget is one database/sql server of the docker stack, with its dialect
// in the storage's own type.
type sqlTarget struct {
	livebackend.SQLTarget
	name    string
	dialect sqldb.Dialect
}

func sqlTargets() []sqlTarget {
	var out []sqlTarget
	for _, t := range livebackend.SQLTargets() {
		out = append(out, sqlTarget{SQLTarget: t, name: t.Name, dialect: sqldb.Dialect(t.Dialect)})
	}
	return out
}

// open connects to the server and returns a throwaway table name, dropped on
// cleanup.
func (s sqlTarget) open(tb testing.TB) (*sql.DB, string) {
	tb.Helper()
	db := s.Open(tb)
	table := livebackend.SQLTable(tb, db, "rq")
	return db, table
}

// store builds a store over a throwaway table, created with EnsureSchema
// (twice, to prove idempotence), and returns it with its database and table.
func (s sqlTarget) store(tb testing.TB, opts ...sqldb.Option) (*sqldb.Store[storagetest.Payload], *sql.DB, string) {
	tb.Helper()
	db, table := s.open(tb)
	store, err := sqldb.New[storagetest.Payload](db, s.dialect, append([]sqldb.Option{sqldb.WithTableName(table)}, opts...)...)
	require.NoError(tb, err)
	require.NoError(tb, store.EnsureSchema(tb.Context()))
	require.NoError(tb, store.EnsureSchema(tb.Context()))
	return store, db, table
}

// TestSQLStorageContract runs the storage contract suite on every SQL server,
// each contract over its own table.
func TestSQLStorageContract(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			storagetest.Run(t, func(tb testing.TB) storagetest.Storage {
				s, _, _ := target.store(tb, sqldb.WithResolvedRetention(7*24*time.Hour))
				return s
			})
		})
	}
}

func TestSQLReopenReplacesOldPayloadSchema(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			old, db, table := target.store(t)
			ctx := t.Context()
			first, err := old.Mark(ctx, storagetest.Pending("old", storagetest.T0))
			require.NoError(t, err)
			expired, err := old.Expire(ctx, first.Key, first.Generation, first.Since, first.Since)
			require.NoError(t, err)
			require.True(t, expired)

			current, err := sqldb.New[string](db, target.dialect, sqldb.WithTableName(table))
			require.NoError(t, err)
			item := retryqueue.Item[string]{Key: first.Key, Generation: first.Generation, Since: first.Since, Payload: "replacement"}
			tx, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = tx.Rollback() })
			reopened, err := current.Reopen(sqldb.WithTx(ctx, tx), item)
			require.NoError(t, err)
			require.Equal(t, first.Since.Add(time.Nanosecond), reopened.Since)
			require.Equal(t, item.Payload, reopened.Payload)
			require.NoError(t, tx.Rollback())
			expiredItems, err := old.ListByState(ctx, retryqueue.StateExpired, 1)
			require.NoError(t, err)
			require.Len(t, expiredItems, 1)
			require.Equal(t, first.Payload, expiredItems[0].Payload)

			committed, err := current.Reopen(ctx, item)
			require.NoError(t, err)
			require.Equal(t, reopened, committed)
			pending, err := current.ListByState(ctx, retryqueue.StatePending, 1)
			require.NoError(t, err)
			require.Equal(t, []retryqueue.Item[string]{committed}, pending)
		})
	}
}

// TestSQLPurgeExpired asserts PurgeExpired deletes only resolved items past
// the retention, measured against the database clock, and nothing without a
// retention.
func TestSQLPurgeExpired(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			s, db, table := target.store(t, sqldb.WithResolvedRetention(time.Hour))
			ctx, t0 := t.Context(), storagetest.T0
			for _, key := range []string{"old", "recent", "pending", "expired", "zero"} {
				_, err := s.Mark(ctx, storagetest.Pending(key, t0))
				require.NoError(t, err)
			}
			require.NoError(t, s.Resolve(ctx, "old", 1, t0.Add(-2*time.Hour)))
			require.NoError(t, s.Resolve(ctx, "recent", 1, t0.Add(-time.Minute)))
			require.NoError(t, s.Resolve(ctx, "zero", 1, time.Time{}))
			expired, err := s.Expire(ctx, "expired", 1, t0, t0.Add(-48*time.Hour))
			require.NoError(t, err)
			require.True(t, expired)

			n, err := s.PurgeExpired(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(1), n)
			resolvedItems, err := s.ListByState(ctx, retryqueue.StateResolved, 0)
			require.NoError(t, err)
			require.Equal(t, []string{"recent", "zero"}, itemKeys(resolvedItems), "an item resolved at the zero time is kept")

			keep, err := sqldb.New[storagetest.Payload](db, target.dialect, sqldb.WithTableName(table))
			require.NoError(t, err)
			n, err = keep.PurgeExpired(ctx)
			require.NoError(t, err)
			require.Zero(t, n, "without a retention nothing is purged")
		})
	}
}

// TestSQLPurgeExpiredChunks purges a backlog larger than one DELETE.
func TestSQLPurgeExpiredChunks(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			s, _, _ := target.store(t, sqldb.WithResolvedRetention(time.Second))
			ctx, t0 := t.Context(), storagetest.T0
			const backlog = 2100
			for i := range backlog {
				key := fmt.Sprintf("k%05d", i)
				_, err := s.Mark(ctx, storagetest.Pending(key, t0))
				require.NoError(t, err)
				require.NoError(t, s.Resolve(ctx, key, 1, t0.Add(-time.Hour)))
			}
			n, err := s.PurgeExpired(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(backlog), n)
		})
	}
}

// TestSQLBackgroundPurge asserts the purge loop of WithPurgeInterval deletes
// a resolved item past the retention without a PurgeExpired call, keeps one
// within it, and stops on Close.
func TestSQLBackgroundPurge(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			s, _, _ := target.store(t, sqldb.WithResolvedRetention(time.Hour), sqldb.WithPurgeInterval(20*time.Millisecond))
			ctx, t0 := t.Context(), storagetest.T0
			for _, key := range []string{"old", "recent"} {
				_, err := s.Mark(ctx, storagetest.Pending(key, t0))
				require.NoError(t, err)
			}
			require.NoError(t, s.Resolve(ctx, "old", 1, t0.Add(-2*time.Hour)))
			require.NoError(t, s.Resolve(ctx, "recent", 1, t0))

			require.Eventually(t, func() bool {
				items, err := s.ListByState(ctx, retryqueue.StateResolved, 0)
				return err == nil && slices.Equal(itemKeys(items), []string{"recent"})
			}, 5*time.Second, 20*time.Millisecond, "the background purge deletes the item past the retention")

			closeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			require.NoError(t, s.Close(closeCtx))
			require.NoError(t, s.Close(closeCtx), "Close is idempotent")
		})
	}
}

// TestSQLMoveJoinsTransaction asserts Move runs on the transaction WithTx
// carries: rolled back, the item stays under its old key; committed, it reads
// under the new one.
func TestSQLMoveJoinsTransaction(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			s, db, _ := target.store(t)
			ctx := t.Context()
			_, err := s.Mark(ctx, storagetest.Pending("old", storagetest.T0))
			require.NoError(t, err)

			tx, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			require.NoError(t, s.Move(sqldb.WithTx(ctx, tx), "old", "new"))
			require.NoError(t, tx.Rollback())
			require.Equal(t, []string{"old"}, pendingKeys(t, s))

			tx, err = db.BeginTx(ctx, nil)
			require.NoError(t, err)
			require.ErrorIs(t, s.Move(sqldb.WithTx(ctx, tx), "missing", "new"), retryqueue.ErrItemNotFound)
			require.NoError(t, s.Move(sqldb.WithTx(ctx, tx), "old", "new"), "a refusal leaves the transaction usable")
			require.NoError(t, tx.Commit())
			require.Equal(t, []string{"new"}, pendingKeys(t, s))
		})
	}
}

// TestSQLDeleteObsoleteSeesNewerCommit asserts DeleteObsolete inside a
// caller transaction reports a newer generation committed by another
// connection after the transaction's snapshot: on MySQL at REPEATABLE READ a
// plain lookup would read the snapshot's older row and report none.
func TestSQLDeleteObsoleteSeesNewerCommit(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			s, db, _ := target.store(t)
			ctx := t.Context()
			_, err := s.Mark(ctx, storagetest.Pending("k", storagetest.T0))
			require.NoError(t, err)

			var opts *sql.TxOptions
			if target.dialect == sqldb.DialectMySQL {
				opts = &sql.TxOptions{Isolation: sql.LevelRepeatableRead}
			}
			tx, err := db.BeginTx(ctx, opts)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			txCtx := sqldb.WithTx(ctx, tx)
			// A consistent read fixes the transaction's snapshot at generation 1.
			_, err = s.ListByState(txCtx, retryqueue.StatePending, 10)
			require.NoError(t, err)

			newer := storagetest.Pending("k", storagetest.T0)
			newer.Generation = 2
			_, err = s.Reopen(ctx, newer)
			require.NoError(t, err)

			found, err := s.DeleteObsolete(txCtx, "k", 1)
			if target.name == "mariadb" {
				// MariaDB with innodb_snapshot_isolation refuses the DELETE of
				// a row changed after the snapshot (error 1020): the caller
				// restarts the transaction instead of acting on a stale answer.
				if mysqlErr, ok := errors.AsType[*mysql.MySQLError](err); ok && mysqlErr.Number == 1020 {
					return
				}
			}
			require.NoError(t, err)
			require.True(t, found, "the newer generation committed after the snapshot is reported")
			require.NoError(t, tx.Commit())
		})
	}
}

// TestSQLConcurrentFirstMarkAcrossReplicas races the first Mark of one key
// from several stores over one table, each on its own connection pool, as
// service replicas do: exactly one row is inserted and every replica gets it.
func TestSQLConcurrentFirstMarkAcrossReplicas(t *testing.T) {
	t.Parallel()
	const replicas = 8
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			first, _, table := target.store(t)
			stores := make([]*sqldb.Store[storagetest.Payload], replicas)
			for i := range replicas {
				s, err := sqldb.New[storagetest.Payload](target.Open(t), target.dialect, sqldb.WithTableName(table))
				require.NoError(t, err)
				stores[i] = s
			}

			results := make([]storagetest.Item, replicas)
			errs := make([]error, replicas)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i, s := range stores {
				wg.Go(func() {
					<-start
					in := storagetest.Pending("race", storagetest.T0)
					in.Payload.Note = fmt.Sprintf("replica-%d", i)
					results[i], errs[i] = s.Mark(t.Context(), in)
				})
			}
			close(start)
			wg.Wait()
			for i := range replicas {
				require.NoError(t, errs[i], "replica %d", i)
				require.Equal(t, results[0], results[i], "replica %d", i)
			}
			require.Equal(t, []string{"race"}, pendingKeys(t, first))
		})
	}
}

// TestSQLFactory builds the queue through the factory on each SQL server, with
// the factory creating the schema, and runs one item through its lifecycle.
func TestSQLFactory(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			db, table := target.open(t)
			cfg := retryqueueconfig.Default()
			cfg.Storage = &retryqueueconfig.StorageConfig{
				Type: retryqueueconfig.StorageTypeSQL,
				SQL: &retryqueueconfig.SQLStorageConfig{
					Dialect: string(target.dialect), Table: table, EnsureSchema: true, ResolvedRetention: time.Hour,
				},
			}
			queue, err := retryqueuefactory.New[storagetest.Payload](&cfg).UseSQLDB(db).Build(t.Context())
			require.NoError(t, err)

			ctx, t0 := t.Context(), storagetest.T0
			_, err = queue.Mark(ctx, storagetest.Pending("k", t0))
			require.NoError(t, err)
			require.NoError(t, queue.Resolve(ctx, "k", 1, t0.Add(-2*time.Hour)))
			purger, ok := queue.(interface {
				PurgeExpired(ctx context.Context) (int64, error)
			})
			require.True(t, ok, "the factory returns the sqldb store")
			n, err := purger.PurgeExpired(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(1), n, "the configured retention reaches the store")
		})
	}
}

// pendingKeys lists the keys of the pending items of s.
func pendingKeys(t *testing.T, s *sqldb.Store[storagetest.Payload]) []string {
	t.Helper()
	items, err := s.ListByState(t.Context(), retryqueue.StatePending, 0)
	require.NoError(t, err)
	return itemKeys(items)
}

// itemKeys returns the keys of items in order.
func itemKeys(items []storagetest.Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Key)
	}
	return out
}

func TestSQLScalarPayload(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			db, table := target.open(t)
			s, err := sqldb.New[string](db, target.dialect, sqldb.WithTableName(table))
			require.NoError(t, err)
			require.NoError(t, s.EnsureSchema(t.Context()))
			storagetest.ScalarPayload(t, s, nil, nil)
		})
	}
}
