// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/leadelect"
	"github.com/altessa-s/go-atlas/coord/leadelect/providers"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// Stop on a live leader publishes the loss and then cancels the dispatch
// context. Whichever of the two the dispatcher sees first, the loss callback
// runs once, on a context that Stop does not cancel: it ends by the handler
// timeout, not by the cancellation.
func TestLeader_StopRunsLossCallbackOnADetachedContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// waitForCallback makes the provider's Stop return only once the loss
		// callback is running, so the cancellation lands in the middle of it.
		// Otherwise the loss and the cancellation race.
		waitForCallback bool
		rounds          int
	}{
		{name: "canceled while the callback runs", waitForCallback: true, rounds: 1},
		{name: "canceled as the loss is queued", waitForCallback: false, rounds: 20},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for range tc.rounds {
				prov := &mockProvider{nodeID: "node-1"}
				le := leadelect.New(prov, "election", "node-1", leadelect.WithHandlerTimeout(50*time.Millisecond))

				var (
					became  atomic.Int32
					mu      sync.Mutex
					lostErr []error
				)
				entered := make(chan struct{}, 1)
				le.RegisterOnBecomesLeader(func(context.Context, leadelect.LeaderElector) { became.Add(1) })
				le.RegisterOnLeaderLost(func(ctx context.Context, _ leadelect.LeaderElector) {
					entered <- struct{}{}
					<-ctx.Done()
					mu.Lock()
					defer mu.Unlock()
					lostErr = append(lostErr, ctx.Err())
				})

				var cfg providers.Config
				prov.stopFn = func(context.Context) error {
					require.True(t, trySend(t, cfg.LostCh))
					if tc.waitForCallback {
						<-entered
					}
					return nil
				}
				cfg = startCapturing(t, le, prov)
				require.True(t, trySend(t, cfg.BecameCh))
				testhelpers.WaitFor(t, 5*time.Second, func() bool { return became.Load() == 1 }, "the leader callback did not run")

				require.NoError(t, le.Stop(t.Context()))

				mu.Lock()
				got := lostErr
				mu.Unlock()
				require.Len(t, got, 1, "the loss callback must run exactly once")
				require.ErrorIs(t, got[0], context.DeadlineExceeded, "Stop must not cancel the loss callback")
			}
		})
	}
}
