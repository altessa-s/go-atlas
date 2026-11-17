// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/stretchr/testify/require"
)

func TestPurgeDefaultsAndDisabledInterval(t *testing.T) {
	t.Parallel()
	o := newOptions()
	require.Equal(t, time.Minute, o.purgeInterval)
	require.Equal(t, 1000, o.purgeBatchSize)
	o = newOptions(WithPurgeBatchSize(3), WithPurgeBatchSize(0), WithLogger(nil))
	require.Equal(t, 3, o.purgeBatchSize)
	require.NotNil(t, o.logger)
	db, fake := testhelpers.NewFakeSQL(t, func(q string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(q, "DELETE") {
			return testhelpers.FakeSQLReply{}
		}
		return testhelpers.FakeSQLReply{Affected: 1}
	})
	s, err := New[struct{}](db, DialectMySQL, WithResolvedRetention(time.Hour), WithPurgeInterval(0))
	require.NoError(t, err)
	require.NoError(t, s.Delete(t.Context(), "key"))
	require.Nil(t, s.purgeDone, "zero interval must disable automatic purging")
	n, err := s.PurgeExpired(t.Context())
	require.NoError(t, err)
	require.Zero(t, n)
	require.NotEmpty(t, fake.Calls(), "manual purge remains available")
	require.NoError(t, s.Close(t.Context()))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, s.Close(ctx), "an absent loop satisfies an ended context")
}

func TestPurgePartialCount(t *testing.T) {
	t.Parallel()
	for _, canceled := range []bool{false, true} {
		name := "SQL error"
		if canceled {
			name = "canceled"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			db, fake := testhelpers.NewFakeSQL(t, func(q string, _ []any) testhelpers.FakeSQLReply {
				calls++
				if calls > 1 {
					return testhelpers.FakeSQLReply{Err: context.DeadlineExceeded}
				}
				if canceled {
					cancel()
				}
				return testhelpers.FakeSQLReply{Affected: 2}
			})
			s, err := New[struct{}](db, DialectPostgres, WithResolvedRetention(time.Hour), WithPurgeBatchSize(2))
			require.NoError(t, err)
			n, err := s.PurgeExpired(ctx)
			require.Equal(t, int64(2), n)
			if canceled {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			}
			require.Contains(t, fake.Calls()[0].Query, "LIMIT 2 FOR UPDATE SKIP LOCKED")
		})
	}
}

func TestPurgeStartsOnlyOnWritesOutsideTransaction(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	s, err := New[struct{}](db, DialectPostgres, WithResolvedRetention(time.Hour), WithPurgeInterval(time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close(context.WithoutCancel(t.Context()))) })
	_, err = s.ListDue(t.Context(), time.Now(), 1)
	require.NoError(t, err)
	_, err = s.PurgeExpired(t.Context())
	require.NoError(t, err)
	require.Nil(t, s.purgeDone, "reads and manual purges start no goroutine")
	before := len(fake.Calls())
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	ctx, cancel := context.WithCancel(WithTx(t.Context(), tx))
	require.NoError(t, s.Delete(ctx, "key"))
	cancel()
	testhelpers.WaitFor(t, time.Second, func() bool { return len(fake.Calls()) > before+1 }, "background pass survives caller cancellation")
	require.NoError(t, s.Close(t.Context()))
	for _, call := range fake.Calls()[before+1:] {
		require.False(t, call.InTx, "background purges must not join the business transaction")
	}
	ctx, cancelEnded := context.WithCancel(t.Context())
	cancelEnded()
	require.NoError(t, s.Close(ctx), "an exited loop satisfies an ended context")
}
