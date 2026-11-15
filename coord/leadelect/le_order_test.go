// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/leadelect"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// transitionLog records the callbacks a Leader runs, in order.
type transitionLog struct {
	mu     sync.Mutex
	events []string
}

func (l *transitionLog) add(event string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

func (l *transitionLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.events)
}

// A loss followed by a new term, queued while a callback is still running,
// reaches the callbacks in that order: the node ends as leader. Stop then ends
// the term with exactly one loss, although the provider publishes none.
func TestLeader_QueuedLossAndNewTermRunInOrder(t *testing.T) {
	t.Parallel()

	// The dispatcher picks at random among ready channels, so one round would
	// pass half the time without the ordering.
	for range 40 {
		prov := &mockProvider{nodeID: "node-1"}
		le := leadelect.New(prov, "election", "node-1")

		var log transitionLog
		entered := make(chan struct{}, 1)
		release := make(chan struct{})
		le.RegisterOnBecomesLeader(func(ctx context.Context, _ leadelect.LeaderElector) {
			log.add("became")
			select {
			case entered <- struct{}{}:
				select { // only the first term's callback waits
				case <-release:
				case <-ctx.Done():
				}
			default:
			}
		})
		le.RegisterOnLeaderLost(func(context.Context, leadelect.LeaderElector) { log.add("lost") })

		cfg := startCapturing(t, le, prov)
		require.True(t, trySend(t, cfg.BecameCh))
		<-entered

		// The dispatcher is busy: the next two transitions queue up behind it.
		require.True(t, trySend(t, cfg.LostCh))
		require.True(t, trySend(t, cfg.BecameCh))
		close(release)

		want := []string{"became", "lost", "became"}
		testhelpers.WaitFor(t, 5*time.Second, func() bool { return len(log.snapshot()) >= len(want) }, "queued transitions were not dispatched")
		require.Equal(t, want, log.snapshot())

		require.NoError(t, le.Stop(t.Context()))
		require.Equal(t, append(want, "lost"), log.snapshot(), "Stop on a live leader must run the loss callbacks exactly once")
		require.NoError(t, le.Stop(t.Context()))
		require.Equal(t, append(want, "lost"), log.snapshot())
	}
}
