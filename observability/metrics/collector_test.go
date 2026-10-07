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
