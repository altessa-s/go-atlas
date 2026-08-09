// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// watchableStore is a recordingStore that also pushes notifications, so the
// tests can drive Outbox.Watch without a live MongoDB.
type watchableStore struct {
	recordingStore

	// batches is served one element per fetch, so a test can hand the
	// dispatcher a full batch followed by a short one.
	batches [][]Event
	served  atomic.Int64

	// drive is called with the notify callback once Watch starts, letting the
	// test decide when a notification fires. It returns the error Watch reports.
	drive func(ctx context.Context, notify func()) error
}

func (s *watchableStore) FetchUnprocessedEvents(context.Context, uint32) ([]Event, error) {
	i := s.served.Add(1) - 1
	if int(i) >= len(s.batches) {
		return nil, nil
	}
	return s.batches[i], nil
}

func (s *watchableStore) Watch(ctx context.Context, notify func()) error {
	return s.drive(ctx, notify)
}

// blockingStore implements Watcher but never notifies, so Watch stays parked
// until its context ends.
type blockingStore struct{ watchableStore }

// A store with no Watcher must be reported as a missing capability rather than
// a failure: dispatch still works, only on the poll cycle.
func TestWatch_UnsupportedStore(t *testing.T) {
	t.Parallel()

	ob := New(&recordingStore{}, noopHandler)
	require.ErrorIs(t, ob.Watch(t.Context()), ErrWatchUnsupported)
}

// A notification must actually run a dispatch cycle — that is the whole point
// of the watcher.
func TestWatch_NotificationDispatches(t *testing.T) {
	t.Parallel()

	var dispatched atomic.Int64
	dispatchedOnce := make(chan struct{})
	store := &watchableStore{
		batches: [][]Event{{{Id: "e1", Key: "orders.created"}}},
	}
	store.drive = func(ctx context.Context, notify func()) error {
		notify()
		<-ctx.Done()
		return ctx.Err()
	}

	tc := testhelpers.NewTestCollector()
	ob := New(store, func(context.Context, Event) error {
		if dispatched.Add(1) == 1 {
			close(dispatchedOnce)
		}
		return nil
	}, WithCollector(tc))

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	watchDone := make(chan error, 1)
	go func() { watchDone <- ob.Watch(ctx) }()

	select {
	case <-dispatchedOnce:
	case <-time.After(5 * time.Second):
		t.Fatal("notification did not trigger a dispatch cycle")
	}

	cancel()
	require.NoError(t, <-watchDone)
	require.InDelta(t, 1, testhelpers.GetCounterValue(t, tc, "test_outbox_watch_notifications_total"), 0.001)
}

// A full batch means the store held more work than one cycle could take, and no
// further insert will arrive to say so. Without the re-arm the remainder would
// sit until the next poll tick.
func TestWatch_FullBatchRearmsDrain(t *testing.T) {
	t.Parallel()

	const batchSize = 2

	full := []Event{{Id: "e1", Key: "k"}, {Id: "e2", Key: "k2"}}
	short := []Event{{Id: "e3", Key: "k3"}}

	var dispatched atomic.Int64
	allDispatched := make(chan struct{})
	store := &watchableStore{batches: [][]Event{full, short}}
	store.drive = func(ctx context.Context, notify func()) error {
		notify() // A single notification must drain both batches.
		<-ctx.Done()
		return ctx.Err()
	}

	ob := New(store, func(context.Context, Event) error {
		if dispatched.Add(1) == int64(len(full)+len(short)) {
			close(allDispatched)
		}
		return nil
	}, WithEventsBatchSize(batchSize))

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	watchDone := make(chan error, 1)
	go func() { watchDone <- ob.Watch(ctx) }()

	select {
	case <-allDispatched:
	case <-time.After(5 * time.Second):
		t.Fatalf("a full batch did not re-arm the drain: dispatched %d of %d",
			dispatched.Load(), len(full)+len(short))
	}

	cancel()
	require.NoError(t, <-watchDone)
}

// Cancellation is a clean shutdown, not an error the caller has to filter out.
func TestWatch_CancellationReturnsNil(t *testing.T) {
	t.Parallel()

	store := &blockingStore{}
	store.drive = func(ctx context.Context, _ func()) error {
		<-ctx.Done()
		return ctx.Err()
	}

	ob := New(store, noopHandler)

	ctx, cancel := context.WithCancel(t.Context())
	watchDone := make(chan error, 1)
	go func() { watchDone <- ob.Watch(ctx) }()

	cancel()
	select {
	case err := <-watchDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Watch did not return after cancellation")
	}
}

// A watcher that dies for its own reasons must surface that to the caller,
// who is the only one who can decide whether to restart it or alert.
func TestWatch_PropagatesStoreError(t *testing.T) {
	t.Parallel()

	watchErr := errors.New("change stream permanently unavailable")
	store := &watchableStore{}
	store.drive = func(context.Context, func()) error { return watchErr }

	ob := New(store, noopHandler)
	require.ErrorIs(t, ob.Watch(t.Context()), watchErr)
}
