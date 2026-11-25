// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
)

func TestStatusPushInvalidatesCachedReadiness(t *testing.T) {
	t.Parallel()
	for _, scoped := range []bool{false, true} {
		name := "unscoped"
		if scoped {
			name = "scoped"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := health.New(health.WithStatusCacheTTL(time.Hour))
			t.Cleanup(c.Close)
			var status atomic.Int32
			status.Store(int32(health.StatusServing))
			checker := health.Func(func(context.Context) health.ServingStatus {
				return health.ServingStatus(status.Load())
			})
			notify := func(s health.ServingStatus) { c.NotifyStatusChange("db", s) }
			if scoped {
				var remove func()
				notify, remove = c.RegisterScopedService("db", checker)
				t.Cleanup(remove)
			} else {
				c.RegisterService("db", checker)
			}
			require.Equal(t, health.StatusServing, c.CheckStatus(t.Context(), ""))
			status.Store(int32(health.StatusNotServing))
			notify(health.StatusNotServing)
			require.Equal(t, health.StatusNotServing, c.CheckStatus(t.Context(), "db"))
			require.Equal(t, health.StatusNotServing, c.CheckStatus(t.Context(), ""))
			status.Store(int32(health.StatusServing))
			notify(health.StatusServing)
			require.Equal(t, health.StatusServing, c.CheckStatus(t.Context(), ""))
		})
	}
}

func TestForwardedReporterScopesNotificationsAndRemoval(t *testing.T) {
	t.Parallel()
	c := health.New()
	t.Cleanup(c.Close)
	forwarded := struct{ health.Reporter }{c}
	checker := health.Func(func(context.Context) health.ServingStatus { return health.StatusServing })
	oldNotify, oldRemove := health.RegisterOwnedService(forwarded, "db", checker)
	currentNotify, currentRemove := health.RegisterOwnedService(forwarded, "db", checker)
	t.Cleanup(currentRemove)
	currentNotify(health.StatusServing)
	oldNotify(health.StatusNotServing)
	oldRemove()
	require.Equal(t, health.StatusServing, c.CheckStatus(t.Context(), "db"))
}

func TestUnrelatedPushKeepsNamedRefreshCacheable(t *testing.T) {
	t.Parallel()
	c := health.New(health.WithStatusCacheTTL(time.Minute))
	t.Cleanup(c.Close)
	var checks atomic.Int32
	entered, resume := make(chan struct{}), make(chan struct{})
	c.RegisterService("a", health.Func(func(ctx context.Context) health.ServingStatus {
		if checks.Add(1) == 1 {
			close(entered)
			select {
			case <-resume:
			case <-ctx.Done():
				return health.StatusNotServing
			}
		}
		return health.StatusServing
	}))
	_, remove := c.RegisterScopedService("b", health.Func(func(context.Context) health.ServingStatus {
		return health.StatusServing
	}))
	t.Cleanup(remove)
	result := make(chan health.ServingStatus, 1)
	go func() { result <- c.CheckStatus(t.Context(), "a") }()
	select {
	case <-entered:
		c.NotifyStatusChange("b", health.StatusNotServing)
	case <-time.After(time.Second):
		require.FailNow(t, "named check did not start")
	}
	close(resume)
	require.Equal(t, health.StatusServing, <-result)
	require.Equal(t, health.StatusServing, c.CheckStatus(t.Context(), "a"))
	require.EqualValues(t, 1, checks.Load())
}

type prefixReporter struct {
	*health.Coordinator
	registered, notified, removed int
}

func (p *prefixReporter) RegisterService(name string, checker health.Checker) {
	p.registered++
	p.Coordinator.RegisterService("my/"+name, checker)
}

func (p *prefixReporter) NotifyStatusChange(name string, status health.ServingStatus) {
	p.notified++
	p.Coordinator.NotifyStatusChange("my/"+name, status)
}

func (p *prefixReporter) UnregisterService(name string) {
	p.removed++
	p.Coordinator.UnregisterService("my/" + name)
}

func TestOwnedServiceHonorsReporterOverrides(t *testing.T) {
	t.Parallel()
	reporter := &prefixReporter{Coordinator: health.New()}
	t.Cleanup(reporter.Close)
	checker := health.Func(func(context.Context) health.ServingStatus { return health.StatusServing })
	oldNotify, oldRemove := health.RegisterOwnedService(reporter, "db", checker)
	notify, remove := health.RegisterOwnedService(reporter, "db", checker)
	oldNotify(health.StatusDegraded)
	oldRemove()
	notify(health.StatusNotServing)
	require.Equal(t, []string{"my/db"}, slices.Collect(reporter.ListServices()))
	require.Equal(t, health.StatusNotServing, reporter.CheckStatus(t.Context(), "my/db"))
	remove()
	remove()
	require.Empty(t, slices.Collect(reporter.ListServices()))
	require.Equal(t, 2, reporter.registered)
	require.Equal(t, 1, reporter.notified)
	require.Equal(t, 1, reporter.removed)
}

func TestDetachableCheckerRemovesScopedRegistrations(t *testing.T) {
	t.Parallel()
	c := health.New()
	t.Cleanup(c.Close)
	checker := health.Func(func(context.Context) health.ServingStatus { return health.StatusServing })
	d := health.NewDetachableChecker(checker)
	_, remove := c.RegisterScopedService("db", d)
	c.RegisterService("other", d)
	c.RegisterScopedService("other", checker)
	d.Detach()
	remove()
	c.RegisterScopedService("late", d)
	require.Equal(t, []string{"other"}, slices.Collect(c.ListServices()))
}

func TestCycleDoesNotPublishProbeSupersededByPush(t *testing.T) {
	t.Parallel()
	c := health.New(health.WithStatusCacheTTL(time.Hour))
	t.Cleanup(c.Close)
	entered, resume := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	notify, remove := c.RegisterScopedService("db", health.Func(func(ctx context.Context) health.ServingStatus {
		if calls.Add(1) == 1 {
			return health.StatusServing
		}
		close(entered)
		select {
		case <-resume:
		case <-ctx.Done():
		}
		return health.StatusDegraded
	}))
	t.Cleanup(remove)
	sub, err := c.Subscribe(t.Context(), "db")
	require.NoError(t, err)
	t.Cleanup(sub.Close)
	c.TriggerRecheckAll()
	done := make(chan error, 1)
	go func() { done <- c.RunHealthCheckCycle(t.Context()) }()
	<-entered
	notify(health.StatusNotServing)
	close(resume)
	require.NoError(t, <-done)
	require.Equal(t, health.StatusNotServing, <-sub.Updates())
	select {
	case status := <-sub.Updates():
		require.FailNow(t, "stale probe delivered", "got %s", status)
	default:
	}
	require.Equal(t, health.StatusNotServing, c.CheckStatus(t.Context(), "db"))
}
