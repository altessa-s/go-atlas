// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestCleanupLazyStartAndClose(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, func(q string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(q, "DELETE") {
			return testhelpers.FakeSQLReply{}
		}
		return testhelpers.FakeSQLReply{Affected: 1}
	})
	s, err := New(db, DialectPostgres, WithPurgeInterval(time.Millisecond), WithLogger(nil))
	require.NoError(t, err)
	require.Nil(t, s.purgeDone, "New must not start a goroutine")
	require.Empty(t, fake.Calls())
	t.Cleanup(func() { require.NoError(t, s.Close(context.WithoutCancel(t.Context()))) })
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	ctx, cancel := context.WithCancel(WithTx(t.Context(), tx))
	_, _, _, err = s.AttemptLockWithTTL(ctx, "key", nil, time.Minute)
	require.NoError(t, err)
	cancel()
	done := s.purgeDone
	_, _, _, err = s.AttemptLock(t.Context(), "other", nil)
	require.NoError(t, err)
	require.Equal(t, done, s.purgeDone, "start exactly once")
	testhelpers.WaitFor(t, time.Second, func() bool {
		for _, call := range fake.Calls() {
			if strings.HasPrefix(call.Query, "DELETE") {
				require.False(t, call.InTx, "cleanup must not inherit WithTx")
				return true
			}
		}
		return false
	}, "cleanup must survive request cancellation")
	require.NoError(t, s.Close(t.Context()))
	require.NoError(t, s.Close(t.Context()))
	require.Eventually(t, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond, "Close returned before the cleanup goroutine exited")
	calls := len(fake.Calls())
	_, _, _, err = s.AttemptLock(t.Context(), "after-close", nil)
	require.NoError(t, err)
	require.Len(t, fake.Calls(), calls+1, "closed cleanup must not restart")
}

func TestCleanupDisabledAndClosedBeforeStart(t *testing.T) {
	t.Parallel()
	for _, interval := range []time.Duration{0, time.Millisecond} {
		t.Run(interval.String(), func(t *testing.T) {
			t.Parallel()
			db, _ := testhelpers.NewFakeSQL(t, nil)
			s, err := New(db, DialectMySQL, WithPurgeInterval(interval))
			require.NoError(t, err)
			if interval > 0 {
				require.NoError(t, s.Close(t.Context()))
			}
			_, _, _, err = s.AttemptLock(t.Context(), "key", nil)
			require.NoError(t, err)
			require.Nil(t, s.purgeDone)
			require.NoError(t, s.Close(t.Context()))
		})
	}
}

func TestCleanupBatching(t *testing.T) {
	t.Parallel()
	var calls int
	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		calls++
		if calls <= 2 {
			return testhelpers.FakeSQLReply{Affected: DefaultPurgeBatchSize}
		}
		return testhelpers.FakeSQLReply{Affected: 3}
	})
	s, err := New(db, DialectPostgres)
	require.NoError(t, err)
	_, err = s.PurgeExpired(t.Context())
	require.NoError(t, err)
	require.Equal(t, 3, calls, "continue until a batch is not full")
	for _, call := range fake.Calls() {
		require.Equal(t, int64(DefaultPurgeBatchSize), call.Args[0])
	}
}

func TestCleanupCloseDeadline(t *testing.T) {
	t.Parallel()
	entered, unblock := make(chan struct{}), make(chan struct{})
	db, _ := testhelpers.NewFakeSQL(t, func(q string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(q, "DELETE") {
			close(entered)
			<-unblock // Simulate a driver that is slow to honor cancellation.
		}
		return testhelpers.FakeSQLReply{Affected: 1}
	})
	s, err := New(db, DialectMySQL, WithPurgeInterval(time.Millisecond))
	require.NoError(t, err)
	defer func() {
		close(unblock)
		require.NoError(t, s.Close(context.WithoutCancel(t.Context())))
	}()
	_, _, _, err = s.AttemptLock(t.Context(), "key", nil)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		select {
		case <-entered:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond, "cleanup did not start")
	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	require.ErrorIs(t, s.Close(ctx), context.DeadlineExceeded)
}

func TestCleanupErrorsAndPanics(t *testing.T) {
	t.Parallel()
	for _, panicking := range []bool{false, true} {
		name := "error"
		if panicking {
			name = "panic"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			db, _ := testhelpers.NewFakeSQL(t, func(q string, _ []any) testhelpers.FakeSQLReply {
				if strings.HasPrefix(q, "DELETE") {
					calls.Add(1)
					if panicking {
						panic("cleanup driver panic")
					}
					return testhelpers.FakeSQLReply{Err: context.DeadlineExceeded}
				}
				return testhelpers.FakeSQLReply{Affected: 1}
			})
			var logged atomic.Int32
			logger := slog.New(slog.NewTextHandler(testLogWriter{&logged}, nil))
			s, err := New(db, DialectPostgres, WithPurgeInterval(time.Millisecond), WithLogger(logger))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, s.Close(context.WithoutCancel(t.Context()))) })
			_, _, _, err = s.AttemptLock(t.Context(), "key", nil)
			require.NoError(t, err)
			testhelpers.WaitFor(t, time.Second, func() bool { return logged.Load() > 0 }, "cleanup failures must be logged")
			if panicking {
				testhelpers.WaitFor(t, time.Second, func() bool { return calls.Load() >= 2 }, "panics must not stop future sweeps")
				require.NoError(t, s.Close(t.Context()), "recovered panic must release Close waiters")
			} else {
				testhelpers.WaitFor(t, time.Second, func() bool { return calls.Load() >= 2 }, "errors must not stop future sweeps")
			}
		})
	}
}

type testLogWriter struct{ writes *atomic.Int32 }

func (w testLogWriter) Write(p []byte) (int, error) {
	w.writes.Add(1)
	return len(p), nil
}
