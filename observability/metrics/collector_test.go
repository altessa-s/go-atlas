// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters/memory"
)

// blockingRegisterAdapter parks the first Register call until release is closed.
type blockingRegisterAdapter struct {
	*memory.Adapter
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (a *blockingRegisterAdapter) Register(desc *adapters.Desc) error {
	a.once.Do(func() {
		close(a.entered)
		<-a.release
	})
	return a.Adapter.Register(desc)
}

func TestCollector_HandleNotVisibleBeforeRegistration(t *testing.T) {
	t.Parallel()

	adapter := &blockingRegisterAdapter{
		Adapter: memory.New(),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	c := metrics.New(metrics.WithAdapter(adapter), metrics.WithServiceName("svc"))
	opts := metrics.MetricOpts{Name: "jobs_total", Help: "test"}

	var wg sync.WaitGroup
	wg.Go(func() { c.Counter(opts) })
	<-adapter.entered // registration is in flight

	secondDone := make(chan struct{})
	wg.Go(func() {
		defer close(secondDone)
		c.Counter(opts).Inc()
	})
	// Bounded coverage: on the old code the second caller got the handle and
	// recorded (and lost) the increment while Register was still blocked.
	select {
	case <-secondDone:
	case <-time.After(100 * time.Millisecond):
	}

	close(adapter.release)
	wg.Wait()

	require.InDelta(t, 1.0, adapter.CounterValue("svc_jobs_total", nil), 0)
}

func TestCollector_WithSubsystemHidesLifecycle(t *testing.T) {
	t.Parallel()

	c := metrics.New(metrics.WithAdapter(memory.New()), metrics.WithServiceName("svc"))
	t.Cleanup(func() { require.NoError(t, c.Shutdown(t.Context())) })

	scoped := c.WithSubsystem("db")
	nested := scoped.WithSubsystem("pool")
	for _, m := range []any{scoped, nested} {
		_, isCollector := m.(metrics.Collector)
		require.False(t, isCollector, "scoped meter must not expose Shutdown/ForceFlush of the shared parent")
	}

	// Instruments created through the scoped meter keep working.
	metrics.MustCounter(nested, metrics.MetricOpts{Name: "acquired_total", Help: "test"}).Inc()
}

// lifecycleAdapter records flush and close calls on a real recording backend.
type lifecycleAdapter struct {
	*memory.Adapter
	calls []string
}

func (a *lifecycleAdapter) Flush() error {
	a.calls = append(a.calls, "flush")
	return nil
}

func (a *lifecycleAdapter) Close() error {
	a.calls = append(a.calls, "close")
	return nil
}

func TestOptionalAdapterLifecycle(t *testing.T) {
	t.Parallel()
	for _, multi := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			expose func(*lifecycleAdapter) adapters.Adapter
			want   []string
		}{
			{"record_only", func(a *lifecycleAdapter) adapters.Adapter {
				return struct{ adapters.Adapter }{a}
			}, nil},
			{"flush_only", func(a *lifecycleAdapter) adapters.Adapter {
				return flushOnlyAdapter{Adapter: a, flush: a.Flush}
			}, []string{"flush", "flush"}},
			{"close_only", func(a *lifecycleAdapter) adapters.Adapter {
				return closeOnlyAdapter{Adapter: a, close: a.Close}
			}, []string{"close"}},
			{"both", func(a *lifecycleAdapter) adapters.Adapter { return a }, []string{"flush", "flush", "close"}},
		} {
			name := tc.name
			if multi {
				name = "multi/" + name
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				probe := &lifecycleAdapter{Adapter: memory.New()}
				a := tc.expose(probe)
				if multi {
					a = adapters.NewMultiAdapter(struct{ adapters.Adapter }{memory.New()}, a)
				}
				c := metrics.New(metrics.WithAdapter(a))
				require.NoError(t, c.ForceFlush(t.Context()))
				require.NoError(t, c.Shutdown(t.Context()))
				require.NoError(t, c.Shutdown(t.Context()))
				require.ErrorIs(t, c.ForceFlush(t.Context()), metrics.ErrCollectorShutdown)
				require.Equal(t, tc.want, probe.calls)
			})
		}
	}
}

type flushOnlyAdapter struct {
	adapters.Adapter
	flush func() error
}

func (a flushOnlyAdapter) Flush() error { return a.flush() }

type closeOnlyAdapter struct {
	adapters.Adapter
	close func() error
}

func (a closeOnlyAdapter) Close() error { return a.close() }
