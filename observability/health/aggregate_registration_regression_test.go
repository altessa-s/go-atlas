// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
)

func TestAggregateRevalidatesCompletedRegistrations(t *testing.T) {
	t.Parallel()
	for _, api := range []string{"overall", "cached overall", "watcher cycle"} {
		t.Run(api, func(t *testing.T) {
			t.Parallel()
			c := health.New(health.WithMaxConcurrentHealthChecks(1), health.WithCheckTimeout(10*time.Second),
				health.WithStatusCacheTTL(time.Minute))
			t.Cleanup(c.Close)
			var subscription health.Subscriber
			if api == "watcher cycle" {
				var err error
				subscription, err = c.Subscribe(t.Context(), "")
				require.NoError(t, err)
				t.Cleanup(subscription.Close)
				require.Equal(t, health.StatusServing, subscription.InitialStatus())
			}
			completed := make(chan string, 1)
			blocked, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			for _, name := range []string{"a", "b"} {
				c.RegisterService(name, health.Func(func(ctx context.Context) health.ServingStatus {
					if calls.Add(1) == 1 {
						completed <- name
						return health.StatusServing
					}
					close(blocked)
					select {
					case <-release:
					case <-ctx.Done():
					}
					return health.StatusServing
				}))
			}
			var status health.ServingStatus
			var cycleErr error
			done := make(chan struct{})
			go func() {
				defer close(done)
				switch api {
				case "overall":
					status = c.CheckHealth(t.Context())
				case "cached overall":
					status = c.CheckStatus(t.Context(), "")
				case "watcher cycle":
					cycleErr = c.RunHealthCheckCycle(t.Context())
				}
			}()
			<-blocked
			c.RegisterService(<-completed, health.Func(func(context.Context) health.ServingStatus { return health.StatusNotServing }))
			close(release)
			<-done
			require.NoError(t, cycleErr)
			require.Equal(t, int32(2), calls.Load())
			require.Equal(t, 0, c.GetMetrics()["cached_statuses"], "the changed aggregate must not populate the cache")
			if subscription != nil {
				select {
				case status = <-subscription.Updates():
				default:
					require.FailNow(t, "the watcher did not receive the validated aggregate")
				}
			}
			require.Equal(t, health.StatusNotServing, status)
		})
	}
}
