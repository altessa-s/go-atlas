// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
)

// hungChecker blocks until its context ends, like a dependency that stopped
// answering.
func hungChecker(calls *atomic.Int64) health.Func {
	return func(ctx context.Context) health.ServingStatus {
		calls.Add(1)
		<-ctx.Done()
		return health.StatusNotServing
	}
}

// TestCheckHealth_HungCheckerDoesNotStarveOthers pins that every checker runs
// under its own timeout even when another one hangs.
func TestCheckHealth_HungCheckerDoesNotStarveOthers(t *testing.T) {
	t.Parallel()

	c := health.New(health.WithCheckTimeout(100 * time.Millisecond))
	t.Cleanup(c.Close)

	var hung, healthy atomic.Int64
	c.RegisterService("hung", hungChecker(&hung))
	for i := range 5 {
		c.RegisterService(fmt.Sprintf("ok-%d", i), health.Func(func(ctx context.Context) health.ServingStatus {
			if ctx.Err() != nil {
				return health.StatusNotServing
			}
			healthy.Add(1)
			return health.StatusServing
		}))
	}

	start := time.Now()
	require.Equal(t, health.StatusNotServing, c.CheckHealth(t.Context()))

	require.Less(t, time.Since(start), 5*time.Second, "the hung checker must be bounded by the check timeout")
	require.Equal(t, int64(5), healthy.Load(), "the healthy checkers must run with a live context")
}

// TestHealth_BoundedWithoutCallerDeadline pins that Health returns although
// the caller's context carries no deadline.
func TestHealth_BoundedWithoutCallerDeadline(t *testing.T) {
	t.Parallel()

	c := health.New(health.WithCheckTimeout(100 * time.Millisecond))
	t.Cleanup(c.Close)

	var calls atomic.Int64
	c.RegisterService("hung", hungChecker(&calls))

	done := make(chan error, 1)
	go func() { done <- c.Health(t.Context()) }()

	select {
	case err := <-done:
		require.ErrorIs(t, err, health.ErrNotServing)
	case <-time.After(10 * time.Second):
		require.FailNow(t, "Health did not return")
	}
}

// TestCheckStatus_ExpiredCacheRefreshIsShared pins that callers finding the
// cached status expired at the same time run the checkers once.
func TestCheckStatus_ExpiredCacheRefreshIsShared(t *testing.T) {
	t.Parallel()

	c := health.New()
	t.Cleanup(c.Close)

	const callers = 16
	var calls atomic.Int64
	entered := make(chan struct{}, callers)
	release := make(chan struct{})
	c.RegisterService("db", health.Func(func(context.Context) health.ServingStatus {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		return health.StatusServing
	}))

	var wg sync.WaitGroup
	statuses := make([]health.ServingStatus, callers)
	for i := range callers {
		wg.Go(func() { statuses[i] = c.CheckStatus(t.Context(), "db") })
	}

	<-entered
	// Give the remaining callers time to reach the refresh while it is held.
	time.Sleep(200 * time.Millisecond)
	close(release)
	wg.Wait()

	require.Equal(t, int64(1), calls.Load(), "concurrent callers must share one refresh")
	for _, st := range statuses {
		require.Equal(t, health.StatusServing, st)
	}
}
