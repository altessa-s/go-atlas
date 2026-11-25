// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A wake-up still pending when the context ends must not start a cycle: the
// fetch runs detached from cancellation and locks a batch, while the handlers
// run under the canceled context and attempt none of it, so the events would
// stay locked until the unlock sweeper reclaims them.
func TestWatch_PendingWakeAfterShutdownFetchesNothing(t *testing.T) {
	t.Parallel()

	notified := make(chan struct{})
	release := make(chan struct{})
	store := &watchableStore{batches: [][]Event{{{Id: "e1", Key: "orders.created"}}}}
	store.drive = func(ctx context.Context, notify func()) error {
		<-ctx.Done()
		notify() // The wake-up arrives after shutdown, before the stream ends.
		close(notified)
		<-release
		return ctx.Err()
	}

	ob := New(store, noopHandler)

	ctx, cancel := context.WithCancel(t.Context())
	watchDone := make(chan error, 1)
	go func() { watchDone <- ob.Watch(ctx) }()

	cancel()
	<-notified
	require.Never(t, func() bool { return store.served.Load() > 0 }, 200*time.Millisecond, 10*time.Millisecond,
		"a wake-up after shutdown must not fetch and lock a batch")

	close(release)
	require.NoError(t, <-watchDone)
	require.Zero(t, store.served.Load())
}

func TestStartWatchSharesOneLifetime(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		stopFirst bool
		calls     int32
	}{
		{"stopped before startup", true, 0}, {"repeated startup", false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			ob := New(&watchableStore{drive: func(ctx context.Context, _ func()) error {
				calls.Add(1)
				<-ctx.Done()
				return ctx.Err()
			}}, noopHandler)
			if tc.stopFirst {
				ob.StopWatch()
			}
			first := ob.StartWatch(t.Context())
			second := ob.StartWatch(t.Context())
			second()
			first()
			ob.StopWatch()
			require.Equal(t, tc.calls, calls.Load())
		})
	}
}
