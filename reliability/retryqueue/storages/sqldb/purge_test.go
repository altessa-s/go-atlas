// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/retryqueue/storages/sqldb"
)

// purgeCalls counts the purge statements fake received.
func purgeCalls(fake *testhelpers.FakeSQL) int {
	n := 0
	for _, c := range fake.Calls() {
		if strings.Contains(c.Query, "resolved_at <=") {
			n++
		}
	}
	return n
}

// recordingHandler is a slog handler that keeps the messages it receives.
type recordingHandler struct {
	mu   sync.Mutex
	msgs []string
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler            { return h }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.msgs = append(h.msgs, r.Message)
	return nil
}

func (h *recordingHandler) has(msg string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, m := range h.msgs {
		if m == msg {
			return true
		}
	}
	return false
}

// The loop starts with the first Storage call, not in New, purges on its
// interval, and stops for good on Close, which is idempotent.
func TestPurgeLoopLifecycle(t *testing.T) {
	t.Parallel()
	store, fake, _ := newStore(t, sqldb.DialectPostgres, nil,
		sqldb.WithResolvedRetention(time.Hour), sqldb.WithPurgeInterval(5*time.Millisecond))

	time.Sleep(30 * time.Millisecond)
	require.Zero(t, purgeCalls(fake), "New starts no loop")

	require.NoError(t, store.Delete(t.Context(), "k"))
	testhelpers.WaitFor(t, 2*time.Second, func() bool { return purgeCalls(fake) >= 2 }, "the loop purges on its interval")

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	require.NoError(t, store.Close(ctx))
	stopped := purgeCalls(fake)
	require.NoError(t, store.Delete(t.Context(), "k"), "the store stays usable after Close")
	time.Sleep(30 * time.Millisecond)
	require.Equal(t, stopped, purgeCalls(fake), "the loop never starts again")
	require.NoError(t, store.Close(ctx), "Close is idempotent")
}

// Without an interval, or without a retention, no loop runs; Close is a no-op.
func TestPurgeLoopOff(t *testing.T) {
	t.Parallel()
	for name, opts := range map[string][]sqldb.Option{
		"no interval":  {sqldb.WithResolvedRetention(time.Hour), sqldb.WithPurgeInterval(0)},
		"no retention": {sqldb.WithPurgeInterval(time.Millisecond)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store, fake, _ := newStore(t, sqldb.DialectMySQL, nil, opts...)
			require.NoError(t, store.Delete(t.Context(), "k"))
			time.Sleep(20 * time.Millisecond)
			require.Zero(t, purgeCalls(fake))
			require.NoError(t, store.Close(t.Context()))
		})
	}
}

// A store closed before its first Storage call never starts the loop.
func TestPurgeLoopClosedBeforeStart(t *testing.T) {
	t.Parallel()
	store, fake, _ := newStore(t, sqldb.DialectPostgres, nil,
		sqldb.WithResolvedRetention(time.Hour), sqldb.WithPurgeInterval(time.Millisecond))
	require.NoError(t, store.Close(t.Context()))
	require.NoError(t, store.Delete(t.Context(), "k"))
	time.Sleep(20 * time.Millisecond)
	require.Zero(t, purgeCalls(fake))
}

// A failing purge is logged and the loop keeps running; Close waits for a
// purge in progress only within its context.
func TestPurgeLoopFailuresAndCloseDeadline(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	var purges atomic.Int32
	handler := &recordingHandler{}
	store, _, _ := newStore(t, sqldb.DialectPostgres, func(q string, _ []any) testhelpers.FakeSQLReply {
		if !strings.Contains(q, "resolved_at <=") {
			return testhelpers.FakeSQLReply{Affected: 1}
		}
		if purges.Add(1) <= 2 {
			return testhelpers.FakeSQLReply{Err: errBoom}
		}
		<-release // the third purge hangs until the test releases it
		return testhelpers.FakeSQLReply{Affected: 0}
	}, sqldb.WithResolvedRetention(time.Hour), sqldb.WithPurgeInterval(time.Millisecond), sqldb.WithLogger(slog.New(handler)))

	require.NoError(t, store.Delete(t.Context(), "k"))
	testhelpers.WaitFor(t, 2*time.Second, func() bool { return purges.Load() >= 3 }, "the loop survives failing purges")
	require.True(t, handler.has("failed to purge resolved retry items"))

	short, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, store.Close(short), context.DeadlineExceeded, "Close waits only within its context")

	close(release)
	ctx, cancelWait := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancelWait()
	require.NoError(t, store.Close(ctx), "a later Close returns at once")
}

// A panicking purge is recovered and logged, and the loop keeps running.
func TestPurgeLoopRecoversPanics(t *testing.T) {
	t.Parallel()
	var purges atomic.Int32
	handler := &recordingHandler{}
	store, _, _ := newStore(t, sqldb.DialectPostgres, func(q string, _ []any) testhelpers.FakeSQLReply {
		if strings.Contains(q, "resolved_at <=") && purges.Add(1) == 1 {
			panic("driver bug")
		}
		return testhelpers.FakeSQLReply{Affected: 0}
	}, sqldb.WithResolvedRetention(time.Hour), sqldb.WithPurgeInterval(time.Millisecond), sqldb.WithLogger(slog.New(handler)))

	require.NoError(t, store.Delete(t.Context(), "k"))
	testhelpers.WaitFor(t, 2*time.Second, func() bool { return purges.Load() >= 2 }, "the loop survives a panic")
	require.True(t, handler.has("resolved retry items purge panicked"))
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	require.NoError(t, store.Close(ctx))
}
