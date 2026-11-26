// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages/sqldb"
)

type mysqlError struct{ Number uint16 }

func (e *mysqlError) Error() string { return "mysql server error" }

type postgresError struct{ state string }

func (e *postgresError) Error() string    { return "postgres server error" }
func (e *postgresError) SQLState() string { return e.state }

func TestRestartTransaction(t *testing.T) {
	t.Parallel()
	operations := []struct {
		name string
		run  func(context.Context, *sqldb.Storage) error
	}{
		{"attempt", func(ctx context.Context, s *sqldb.Storage) error {
			_, _, _, err := s.AttemptLock(ctx, "key", nil)
			return err
		}},
		{"complete", func(ctx context.Context, s *sqldb.Storage) error { return s.Complete(ctx, "key", nil, nil) }},
		{"complete CAS", func(ctx context.Context, s *sqldb.Storage) error { return s.Complete(ctx, "key", nil, []byte("lock")) }},
		{"steal", func(ctx context.Context, s *sqldb.Storage) error {
			_, _, _, err := s.Steal(ctx, "key", []byte("lock"), nil, 0, 0)
			return err
		}},
		{"release", func(ctx context.Context, s *sqldb.Storage) error { return s.Release(ctx, "key", []byte("lock")) }},
		{"delete", func(ctx context.Context, s *sqldb.Storage) error { return s.Delete(ctx, "key") }},
		{"cleanup", func(ctx context.Context, s *sqldb.Storage) error { _, err := s.PurgeExpired(ctx); return err }},
	}
	for _, tc := range []struct {
		name    string
		err     error
		restart bool
	}{
		{"maria snapshot", &mysqlError{1020}, true},
		{"mysql deadlock", &mysqlError{1213}, true},
		{"postgres serialization", &postgresError{"40001"}, true},
		{"postgres deadlock", &postgresError{"40P01"}, true},
		{"mysql other", &mysqlError{1062}, false},
		{"postgres other", &postgresError{"23505"}, false},
		{"other", errors.New("other failure"), false},
	} {
		for _, op := range operations {
			t.Run(tc.name+"/"+op.name, func(t *testing.T) {
				t.Parallel()
				db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
					return testhelpers.FakeSQLReply{Err: fmt.Errorf("driver: %w", tc.err)}
				})
				s, err := sqldb.New(db, sqldb.DialectPostgres)
				require.NoError(t, err)
				tx, err := db.BeginTx(t.Context(), nil)
				require.NoError(t, err)
				defer func() { _ = tx.Rollback() }()
				for _, joined := range []bool{false, true} {
					ctx := t.Context()
					if joined {
						ctx = sqldb.WithTx(ctx, tx)
					}
					err = op.run(ctx, s)
					require.ErrorIs(t, err, tc.err, "driver error must remain in the chain")
					require.Equal(t, joined && tc.restart, errors.Is(err, sqldb.ErrRestartTransaction))
				}
				require.Len(t, fake.Calls(), 2, "caller-owned transactions must never be retried internally")
				require.Zero(t, fake.Commits())
				require.Zero(t, fake.Rollbacks())
			})
		}
	}
}

func TestRestartTransactionAcquisitionStages(t *testing.T) {
	t.Parallel()
	for _, failAt := range []int{2, 3} {
		t.Run(fmt.Sprint(failAt), func(t *testing.T) {
			t.Parallel()
			calls := 0
			db, _ := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
				calls++
				if calls == failAt {
					return testhelpers.FakeSQLReply{Err: &mysqlError{1020}}
				}
				return testhelpers.FakeSQLReply{}
			})
			s, err := sqldb.New(db, sqldb.DialectMySQL)
			require.NoError(t, err)
			tx, err := db.BeginTx(t.Context(), nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			_, _, _, err = s.AttemptLock(sqldb.WithTx(t.Context(), tx), "key", nil)
			require.ErrorIs(t, err, sqldb.ErrRestartTransaction)
			var driverErr *mysqlError
			require.ErrorAs(t, err, &driverErr)
		})
	}
}

func TestInjectedTransaction(t *testing.T) {
	type txKey struct{}
	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Err: &postgresError{"40001"}}
	})
	store, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTxResolver(func(ctx context.Context) (*sql.Tx, bool) {
		tx, ok := ctx.Value(txKey{}).(*sql.Tx)
		return tx, ok
	}))
	require.NoError(t, err)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	ctx := context.WithValue(t.Context(), txKey{}, tx)
	require.ErrorIs(t, store.Delete(ctx, "key"), sqldb.ErrRestartTransaction)
	require.True(t, fake.Calls()[0].InTx)
	require.Zero(t, fake.Commits())
}
