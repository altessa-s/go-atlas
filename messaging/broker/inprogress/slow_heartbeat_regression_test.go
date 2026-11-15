// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package inprogress_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/messaging/broker/inprogress"
)

type fastHeartbeater struct{ calls atomic.Int64 }

func (h *fastHeartbeater) InProgress() error {
	h.calls.Add(1)
	return nil
}

type blockedHeartbeater struct {
	calls   atomic.Int64
	release chan struct{}
}

func (h *blockedHeartbeater) InProgress() error {
	h.calls.Add(1)
	<-h.release
	return nil
}

// TestManager_SlowHeartbeatDoesNotDelayOthers pins that one stuck InProgress
// call neither holds the tick nor keeps the other heartbeats from being sent.
func TestManager_SlowHeartbeatDoesNotDelayOthers(t *testing.T) {
	t.Parallel()

	mgr := inprogress.New(inprogress.WithHeartbeatTimeout(100 * time.Millisecond))

	slow := &blockedHeartbeater{release: make(chan struct{})}
	t.Cleanup(func() { close(slow.release) })
	defer mgr.Register(slow, 0)()

	fast := make([]*fastHeartbeater, 64)
	for i := range fast {
		fast[i] = &fastHeartbeater{}
		defer mgr.Register(fast[i], 0)()
	}

	tick := func() {
		t.Helper()
		done := make(chan error, 1)
		go func() { done <- mgr.RunTickCycle(t.Context()) }()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(10 * time.Second):
			require.FailNow(t, "the tick is held by a stuck heartbeat")
		}
	}

	tick()
	for _, h := range fast {
		require.Equal(t, int64(1), h.calls.Load(), "a heartbeat was not sent because of a stuck one")
	}

	// The stuck call is still outstanding: the next tick must not pile a
	// second one on top of it.
	tick()
	require.Equal(t, int64(1), slow.calls.Load())
	for _, h := range fast {
		require.Equal(t, int64(2), h.calls.Load())
	}
}
