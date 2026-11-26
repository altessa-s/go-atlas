// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observe

import (
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"

	httpclient "github.com/altessa-s/go-atlas-net/http/client"
)

type recordingReporter struct {
	statuses chan health.ServingStatus
}

func (*recordingReporter) RegisterService(string, health.Checker) {}

func (r *recordingReporter) NotifyStatusChange(_ string, status health.ServingStatus) {
	r.statuses <- status
}

type gatedBreakers struct {
	fakeBreakers
	first, second chan struct{}
	calls         atomic.Int32
}

func (b *gatedBreakers) States() []gobreaker.State {
	if b.calls.Add(1) == 1 {
		<-b.first
		return []gobreaker.State{gobreaker.StateOpen}
	}
	<-b.second
	return []gobreaker.State{gobreaker.StateHalfOpen}
}

func TestObserverPublishesFinalBreakerStatus(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		b := &gatedBreakers{first: make(chan struct{}), second: make(chan struct{})}
		reporter := &recordingReporter{statuses: make(chan health.ServingStatus, 2)}
		obs := New(nil, reporter)
		obs.Attach(b)
		defer obs.Close()

		obs.BreakerStateChanged(httpclient.BreakerStateEvent{To: gobreaker.StateOpen})
		synctest.Wait()
		obs.BreakerStateChanged(httpclient.BreakerStateEvent{To: gobreaker.StateHalfOpen})
		synctest.Wait()

		// Let the newer read finish first if it is already running.
		close(b.second)
		synctest.Wait()
		close(b.first)
		synctest.Wait()

		require.NotEmpty(t, reporter.statuses)
		var last health.ServingStatus
		for len(reporter.statuses) > 0 {
			last = <-reporter.statuses
		}
		require.Equal(t, health.StatusDegraded, last)
	})
}

func TestObserverCloseJoinsNotifier(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		b := &gatedBreakers{first: make(chan struct{}), second: make(chan struct{})}
		reporter := &recordingReporter{statuses: make(chan health.ServingStatus, 2)}
		obs := New(nil, reporter)
		obs.Attach(b)

		obs.BreakerStateChanged(httpclient.BreakerStateEvent{To: gobreaker.StateOpen})
		synctest.Wait()
		closed := make(chan struct{})
		go func() {
			obs.Close()
			close(closed)
		}()
		synctest.Wait()
		var closedEarly bool
		select {
		case <-closed:
			closedEarly = true
		default:
		}
		close(b.first)
		close(b.second)
		<-closed
		require.False(t, closedEarly, "close returned while the notifier was reading breaker states")
	})
}

func TestObserverCloseRemovesScopedHealthChecks(t *testing.T) {
	t.Parallel()
	c := health.New()
	defer c.Close()
	makeObserver := func() *Observer {
		obs := New(nil, c, WithPerHostHealthChecks())
		obs.Attach(&fakeBreakers{config: []string{"api.example.com"}})
		return obs
	}
	first, replacement := makeObserver(), makeObserver()
	first.Close()
	require.ElementsMatch(t, []string{"http_client", "http_client.api.example.com"}, slices.Collect(c.ListServices()))
	replacement.Close()
	require.Empty(t, slices.Collect(c.ListServices()))
}
