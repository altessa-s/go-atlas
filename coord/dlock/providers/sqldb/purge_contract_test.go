// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
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
	s, err := New(db, DialectPostgres, WithExpiredCleanup(0), WithPurgeInterval(0))
	require.NoError(t, err)
	_, _ = s.Lock(t.Context(), "key")
	require.Nil(t, s.sweepDone, "zero interval must disable automatic purging")
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
			s, err := New(db, DialectPostgres, WithExpiredCleanup(0), WithPurgeBatchSize(2))
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

func TestPurgeLoopSurvivesPanicAndError(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	var logs bytes.Buffer // Read after Close joins the writer.
	db, _ := testhelpers.NewFakeSQL(t, func(q string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(q, "DELETE") {
			switch calls.Add(1) {
			case 1:
				panic("test driver panic")
			case 2:
				return testhelpers.FakeSQLReply{Err: context.DeadlineExceeded}
			}
			return testhelpers.FakeSQLReply{}
		}
		return testhelpers.FakeSQLReply{}
	})
	s, err := New(db, DialectPostgres, WithExpiredCleanup(0), WithPurgeInterval(time.Millisecond),
		WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close(context.WithoutCancel(t.Context()))) })
	_, _ = s.Lock(t.Context(), "key")
	testhelpers.WaitFor(t, time.Second, func() bool { return calls.Load() >= 3 }, "purges survive panics and errors")
	require.NoError(t, s.Close(t.Context()))
	require.Contains(t, logs.String(), "expired lock purge panicked")
	require.Contains(t, logs.String(), "failed to purge expired locks")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, s.Close(ctx), "an exited loop and closed engine satisfy an ended context")
}
