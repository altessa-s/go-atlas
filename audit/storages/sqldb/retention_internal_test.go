// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/time/clock"
	"github.com/altessa-s/go-atlas/audit"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestRetentionOptions(t *testing.T) {
	t.Parallel()
	o := newOptions(WithTTL(-1), WithPurgeInterval(-1), WithPurgeInterval(0), WithPurgeBatchSize(-1),
		WithPurgeBatchSize(0), WithClock(nil), WithClock((*clock.Fake)(nil)), WithLogger(nil))
	require.Zero(t, o.ttl)
	require.Zero(t, o.purgeInterval)
	require.Equal(t, DefaultPurgeBatchSize, o.purgeBatchSize)
	require.NotNil(t, o.clock)
	require.NotNil(t, o.logger)

	fakeClock := clock.NewFake(time.Now())
	logger := slog.New(slog.DiscardHandler)
	o = newOptions(WithTTL(time.Hour), WithTTL(0), WithTTL(-1), WithPurgeInterval(time.Second), WithPurgeBatchSize(3),
		WithClock(fakeClock), WithLogger(logger))
	require.Equal(t, time.Hour, o.ttl)
	require.Equal(t, time.Second, o.purgeInterval)
	require.Equal(t, 3, o.purgeBatchSize)
	require.Same(t, fakeClock, o.clock)
	require.Same(t, logger, o.logger)
}

func TestPurgeExpiredDisabled(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	s, err := New(db, DialectPostgres)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	n, err := s.PurgeExpired(ctx)
	require.NoError(t, err)
	require.Zero(t, n)
	require.Empty(t, fake.Calls())
	require.NoError(t, s.Close(t.Context()))
}

func TestPurgeExpiredBatches(t *testing.T) {
	t.Parallel()
	for _, d := range []Dialect{DialectPostgres, DialectMySQL} {
		t.Run(string(d), func(t *testing.T) {
			t.Parallel()
			at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			fakeClock := clock.NewFake(at)
			var calls int
			db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
				calls++
				fakeClock.Advance(time.Hour)
				return testhelpers.FakeSQLReply{Affected: []int64{2, 2, 1}[calls-1]}
			})
			s, err := New(db, d, WithTableName("app.events"), WithTTL(time.Hour), WithClock(fakeClock),
				WithPurgeInterval(time.Hour), WithPurgeBatchSize(2))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, s.Close(context.WithoutCancel(t.Context()))) })
			n, err := s.PurgeExpired(t.Context())
			require.NoError(t, err)
			require.EqualValues(t, 5, n)
			statements := fake.Calls()
			require.Len(t, statements, 3)
			want := "DELETE FROM `app`.`events` WHERE ts_ms < ? ORDER BY ts_ms, id LIMIT ?"
			if d == DialectPostgres {
				want = `DELETE FROM "app"."events" WHERE id IN (SELECT id FROM "app"."events"` +
					` WHERE ts_ms < $1 ORDER BY ts_ms, id LIMIT $2 FOR UPDATE SKIP LOCKED)`
			}
			for _, c := range statements {
				require.Equal(t, want, c.Query)
				require.Equal(t, []any{at.Add(-time.Hour).UnixMilli(), int64(2)}, c.Args, "one cutoff for all batches")
				require.False(t, c.InTx, "batches commit independently")
			}
		})
	}
}

func TestPurgeExpiredPartialErrorAndCancellation(t *testing.T) {
	t.Parallel()
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "database error", true: "canceled"}[canceled], func(t *testing.T) {
			t.Parallel()
			boom := errors.New("delete failed")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var calls int
			db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
				calls++
				if calls > 1 {
					return testhelpers.FakeSQLReply{Err: boom}
				}
				if canceled {
					cancel()
				}
				return testhelpers.FakeSQLReply{Affected: 2}
			})
			s, err := New(db, DialectPostgres, WithTTL(time.Hour), WithPurgeInterval(time.Hour), WithPurgeBatchSize(2))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, s.Close(context.WithoutCancel(t.Context()))) })
			n, err := s.PurgeExpired(ctx)
			require.EqualValues(t, 2, n)
			if canceled {
				require.ErrorIs(t, err, context.Canceled)
				require.Len(t, fake.Calls(), 1)
			} else {
				require.ErrorIs(t, err, boom)
				require.Len(t, fake.Calls(), 2)
			}
		})
	}
}

func TestPurgeLoopRecoversAndCloseWaits(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	var logs bytes.Buffer // Read only after Close has joined the writer.
	// Panic from the clock rather than the SQL driver: database/sql drivers
	// do not promise that their connections survive a driver panic.
	c := &panicOnceClock{}
	db, fake := testhelpers.NewFakeSQL(t, func(query string, _ []any) testhelpers.FakeSQLReply {
		if !strings.HasPrefix(query, "DELETE") {
			return testhelpers.FakeSQLReply{Affected: 1}
		}
		if calls.Add(1) == 1 {
			return testhelpers.FakeSQLReply{Err: errors.New("temporary failure")}
		}
		return testhelpers.FakeSQLReply{}
	})
	s, err := New(db, DialectPostgres, WithTTL(time.Hour), WithClock(c), WithPurgeInterval(time.Millisecond),
		WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close(context.WithoutCancel(t.Context()))) })
	require.NoError(t, s.Store(t.Context(), retentionEvent("e1")))
	testhelpers.WaitFor(t, time.Second, func() bool { return calls.Load() >= 2 }, "retention survives panic and SQL error")
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { _ = s.Close(t.Context()) })
	}
	wg.Wait()
	require.Contains(t, logs.String(), "audit events purge panicked")
	require.Contains(t, logs.String(), "failed to purge audit events")
	// Closing retention leaves the caller's database usable.
	require.NoError(t, db.PingContext(t.Context()))
	before := len(fake.Calls())
	_, err = s.PurgeExpired(t.Context())
	require.NoError(t, err)
	require.Len(t, fake.Calls(), before+1)
}

type panicOnceClock struct{ calls atomic.Int64 }

func (c *panicOnceClock) Now() time.Time {
	if c.calls.Add(1) == 1 {
		panic("test retention panic")
	}
	return time.Now()
}

func (*panicOnceClock) Sleep(ctx context.Context, d time.Duration) error {
	return clock.Real().Sleep(ctx, d)
}

func TestCloseCancelsPendingPurge(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	db.SetMaxOpenConns(1)
	s, err := New(db, DialectMySQL, WithTTL(time.Hour), WithPurgeInterval(time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close(context.WithoutCancel(t.Context()))) })
	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	defer conn.Close()
	// The first write starts retention even when its own context is canceled;
	// it cannot use the connection this test holds.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, s.Store(ctx, retentionEvent("canceled")), context.Canceled)
	testhelpers.WaitFor(t, time.Second, func() bool { return db.Stats().WaitCount > 0 }, "purge waiting for connection")
	closed := make(chan struct{})
	go func() {
		_ = s.Close(t.Context())
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel the pending delete")
	}
}

// retentionEvent returns a minimal storable event.
func retentionEvent(id string) *audit.Event {
	return &audit.Event{ID: id, Type: "auth", Action: "login", Timestamp: time.Now()}
}

// The retention loop is lazy: New starts no goroutine, the first StoreBatch
// starts exactly one, reads do not, and Close stops it for good — a later
// Store does not restart it. Without TTL nothing ever starts.
func TestPurgeLoopLifecycle(t *testing.T) {
	t.Parallel()
	var deletes atomic.Int64
	db, _ := testhelpers.NewFakeSQL(t, func(query string, _ []any) testhelpers.FakeSQLReply {
		switch {
		case strings.HasPrefix(query, "DELETE"):
			deletes.Add(1)
		case strings.HasPrefix(query, "SELECT"):
			return testhelpers.FakeSQLReply{Columns: []string{"n"}, Rows: [][]driver.Value{{int64(0)}}}
		}
		return testhelpers.FakeSQLReply{Affected: 1}
	})
	s, err := New(db, DialectPostgres, WithTTL(time.Hour), WithPurgeInterval(time.Millisecond))
	require.NoError(t, err)
	_, err = s.Count(t.Context(), &audit.Query{})
	require.NoError(t, err)
	require.NoError(t, s.StoreBatch(t.Context(), nil))
	require.Zero(t, deletes.Load())

	require.NoError(t, s.Store(t.Context(), retentionEvent("e1")))
	require.NoError(t, s.Store(t.Context(), retentionEvent("e2")))
	testhelpers.WaitFor(t, time.Second, func() bool { return deletes.Load() > 0 }, "retention runs once started")

	require.NoError(t, s.Close(t.Context()))
	before := deletes.Load()
	require.NoError(t, s.Store(t.Context(), retentionEvent("e3")))
	require.Equal(t, before, deletes.Load())
	require.NoError(t, s.Close(t.Context()), "Close is idempotent")

	noTTL, err := New(db, DialectPostgres)
	require.NoError(t, err)
	require.NoError(t, noTTL.Store(t.Context(), retentionEvent("e4")))
	require.NoError(t, noTTL.Close(t.Context()))
}

// Close waits for the loop within its context: a pass that does not return
// in time makes Close report the context's error, and the loop still exits
// once the pass ends.
func TestCloseWaitIsBoundedByContext(t *testing.T) {
	t.Parallel()
	entered, unblock := make(chan struct{}), make(chan struct{})
	var once sync.Once
	db, _ := testhelpers.NewFakeSQL(t, func(query string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(query, "DELETE") {
			once.Do(func() { close(entered) })
			<-unblock
		}
		return testhelpers.FakeSQLReply{Affected: 1}
	})
	s, err := New(db, DialectPostgres, WithTTL(time.Hour), WithPurgeInterval(time.Millisecond))
	require.NoError(t, err)
	require.NoError(t, s.Store(t.Context(), retentionEvent("e1")))
	<-entered

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, s.Close(ctx), context.DeadlineExceeded)
	close(unblock)
	require.NoError(t, s.Close(t.Context()), "a later Close waits for the loop to exit")
}
