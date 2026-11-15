// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// backlogStore is a watchable store whose backlog never runs out: every fetch
// returns a full batch of one new event, so each cycle re-arms the drain.
type backlogStore struct {
	recordingStore
	served atomic.Int64
}

func (s *backlogStore) FetchUnprocessedEvents(context.Context, uint32) ([]Event, error) {
	return []Event{{Id: "e" + strconv.FormatInt(s.served.Add(1), 10), Key: "orders.created"}}, nil
}

// Watch notifies once and then stays parked until its context ends.
func (s *backlogStore) Watch(ctx context.Context, notify func()) error {
	notify()
	<-ctx.Done()
	return ctx.Err()
}

func watchBacklog(t *testing.T, store *backlogStore, handler Handler) (cancel context.CancelFunc, done <-chan error) {
	t.Helper()

	ob := New(store, handler, WithEventsBatchSize(1))
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	watchDone := make(chan error, 1)
	go func() { watchDone <- ob.Watch(ctx) }()
	return cancel, watchDone
}

func failingHandler(context.Context, Event) error { return errors.New("destination down") }

// A handler that fails fast over a backlog must not turn the drain into a hot
// loop against the store: cycles that advance nothing are spaced out.
func TestWatch_StalledDrainPausesBetweenCycles(t *testing.T) {
	t.Parallel()

	store := &backlogStore{}
	cancel, done := watchBacklog(t, store, failingHandler)

	time.Sleep(4 * watchStallBaseDelay)
	cancel()
	require.NoError(t, <-done)

	// Pauses of 100, 150 and 225 ms (plus jitter) fit at most four cycles into
	// the window; an unpaced drain would run thousands.
	cycles := store.served.Load()
	require.GreaterOrEqual(t, cycles, int64(2), "the drain must keep retrying after a pause")
	require.LessOrEqual(t, cycles, int64(5), "a drain that advances nothing must pause between cycles")
}

// Shutdown must not wait out the pause.
func TestWatch_ShutdownInterruptsStallPause(t *testing.T) {
	t.Parallel()

	store := &backlogStore{}
	cancel, done := watchBacklog(t, store, failingHandler)

	// After the fifth cycle the pause in progress is about half a second.
	require.Eventually(t, func() bool { return store.served.Load() >= 5 }, 5*time.Second, time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Watch did not return while pausing between stalled cycles")
	}
}

// A drain that is advancing is not paced: the pause exists for cycles that got
// nowhere, not for a healthy backlog.
func TestWatch_AdvancingDrainDoesNotPause(t *testing.T) {
	t.Parallel()

	store := &backlogStore{}
	cancel, done := watchBacklog(t, store, noopHandler)

	// Twenty paused cycles would take well over ten seconds.
	require.Eventually(t, func() bool { return store.served.Load() >= 20 }, 2*time.Second, time.Millisecond)
	cancel()
	require.NoError(t, <-done)
}
