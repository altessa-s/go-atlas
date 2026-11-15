// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observe

import (
	"errors"
	"io"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/httpx"
	"github.com/altessa-s/go-atlas/observability/health"

	httpclient "github.com/altessa-s/go-atlas-net/http/client"
)

// fakeBreakers is a settable [httpclient.Breakers].
type fakeBreakers struct {
	mu     sync.Mutex
	states []gobreaker.State
	hosts  map[string]gobreaker.State
	config []string
}

func (f *fakeBreakers) States() []gobreaker.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.states)
}

func (f *fakeBreakers) HostState(host string) gobreaker.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hosts[host] // zero value is StateClosed
}

func (f *fakeBreakers) ConfiguredHosts() []string { return f.config }

// newTestHealth constructs a *clientHealth attached to breakers. When coord
// is non-nil attaching also registers the checkers with it.
func newTestHealth(t *testing.T, coord *health.Coordinator, breakers httpclient.Breakers, opts ...Option) *clientHealth {
	t.Helper()
	o := newOptions(append([]Option{
		WithHealthRetryWindow(60 * time.Second),
		WithHealthRetryThreshold(0.2),
		WithHealthRetryMinSamples(10),
	}, opts...)...)
	var reporter health.Reporter
	if coord != nil {
		reporter = coord
	}
	h := newClientHealth(*o, reporter)
	h.attach(breakers)
	return h
}

func TestRetrySlidingWindowRecordAndRate(t *testing.T) {
	t.Parallel()
	w := newRetrySlidingWindow(60*time.Second, 6)

	// 4 plain requests, 1 retried.
	for range 4 {
		w.record(false)
	}
	w.record(true)

	rate, samples := w.rate(time.Now())
	require.Equal(t, uint64(5), samples)
	require.InDelta(t, 0.2, rate, 1e-9)
}

func TestRetrySlidingWindowExpiry(t *testing.T) {
	t.Parallel()
	w := newRetrySlidingWindow(60*time.Second, 6)

	// Force the first observation into a fixed past timestamp.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	w.nowFn = func() time.Time { return base }
	w.record(true)

	// A read 2 windows later should ignore the stale bucket.
	rate, samples := w.rate(base.Add(2 * 60 * time.Second))
	require.Zero(t, samples)
	require.Equal(t, 0.0, rate)

	// A read inside the window still sees the observation.
	rate, samples = w.rate(base.Add(30 * time.Second))
	require.Equal(t, uint64(1), samples)
	require.InDelta(t, 1.0, rate, 1e-9)
}

func TestNewClientHealthNoReporter(t *testing.T) {
	t.Parallel()
	h := newTestHealth(t, nil, &fakeBreakers{config: []string{"api.example.com"}}, WithPerHostHealthChecks())
	require.NotNil(t, h)
	require.Nil(t, h.reporter)
	require.Nil(t, h.aggregate, "aggregate window must not be allocated when the reporter is absent")
	require.Nil(t, h.perHostWindows)

	// Method calls must be cheap noops.
	h.recordRequest("api.example.com", true)
	h.notifyAggregate()
	h.notifyHost("api.example.com")
	require.Equal(t, health.StatusServing, h.CheckHealth(t.Context()))
}

func TestNewClientHealthPerHostFiltersToConfiguredHosts(t *testing.T) {
	t.Parallel()
	coord := health.New()
	defer coord.Close()

	h := newTestHealth(t, coord,
		&fakeBreakers{config: []string{"api.example.com", "backup.example.com"}},
		WithPerHostHealthChecks(),
	)

	require.NotNil(t, h.aggregate)
	require.NotNil(t, h.perHostWindows)
	require.True(t, h.perHostWindows.Contains("api.example.com"))
	require.True(t, h.perHostWindows.Contains("backup.example.com"))

	// Lazy hosts must NOT be registered.
	registered := slices.Collect(coord.ListServices())
	slices.Sort(registered)
	require.Equal(t, []string{
		"http_client",
		"http_client.api.example.com",
		"http_client.backup.example.com",
	}, registered)
}

func TestNewClientHealthPerHostOffSkipsRegistration(t *testing.T) {
	t.Parallel()
	coord := health.New()
	defer coord.Close()

	h := newTestHealth(t, coord, &fakeBreakers{config: []string{"api.example.com"}})

	require.Nil(t, h.perHostWindows)
	require.Equal(t, []string{"http_client"}, slices.Collect(coord.ListServices()))
}

func TestRecordRequestUpdatesPerHostWindow(t *testing.T) {
	t.Parallel()
	coord := health.New()
	defer coord.Close()

	h := newTestHealth(t, coord, &fakeBreakers{config: []string{"api.example.com"}}, WithPerHostHealthChecks())

	for range 4 {
		h.recordRequest("api.example.com", false)
	}
	h.recordRequest("api.example.com", true)
	// Lazy host writes only the aggregate.
	h.recordRequest("lazy.example.com", true)

	rate, samples := h.aggregate.rate(time.Now())
	require.Equal(t, uint64(6), samples)
	require.InDelta(t, 2.0/6.0, rate, 1e-9)

	apiWin, ok := h.perHostWindows.Get("api.example.com")
	require.True(t, ok)
	rate, samples = apiWin.rate(time.Now())
	require.Equal(t, uint64(5), samples)
	require.InDelta(t, 0.2, rate, 1e-9)
}

func TestAggregateStatusRetryRateBelowMinSamplesDoesNotDegrade(t *testing.T) {
	t.Parallel()
	coord := health.New()
	defer coord.Close()

	h := newTestHealth(t, coord, &fakeBreakers{})
	for range 5 {
		h.recordRequest("api.example.com", true)
	}
	// 100% retry rate but only 5 samples (< minSamples=10).
	require.Equal(t, health.StatusServing, h.CheckHealth(t.Context()))
}

func TestAggregateStatusRetryRateAboveThresholdDegrades(t *testing.T) {
	t.Parallel()
	coord := health.New()
	defer coord.Close()

	h := newTestHealth(t, coord, &fakeBreakers{})
	// 12 retried / 8 plain = 60% retry rate, 20 samples.
	for range 12 {
		h.recordRequest("api.example.com", true)
	}
	for range 8 {
		h.recordRequest("api.example.com", false)
	}
	require.Equal(t, health.StatusDegraded, h.CheckHealth(t.Context()))
}

func TestAggregateStatusBreakerStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		states []gobreaker.State
		want   health.ServingStatus
	}{
		{name: "no breakers", want: health.StatusServing},
		{name: "all closed", states: []gobreaker.State{gobreaker.StateClosed}, want: health.StatusServing},
		{name: "all open", states: []gobreaker.State{gobreaker.StateOpen, gobreaker.StateOpen}, want: health.StatusNotServing},
		{name: "some open", states: []gobreaker.State{gobreaker.StateOpen, gobreaker.StateClosed}, want: health.StatusDegraded},
		{name: "half-open", states: []gobreaker.State{gobreaker.StateHalfOpen}, want: health.StatusDegraded},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			coord := health.New()
			defer coord.Close()

			h := newTestHealth(t, coord, &fakeBreakers{states: tc.states})
			require.Equal(t, tc.want, h.CheckHealth(t.Context()))
		})
	}
}

func TestHostStatus(t *testing.T) {
	t.Parallel()
	coord := health.New()
	defer coord.Close()

	fb := &fakeBreakers{config: []string{"api.example.com"}, hosts: map[string]gobreaker.State{}}
	h := newTestHealth(t, coord, fb, WithPerHostHealthChecks())
	checker := &perHostChecker{parent: h, host: "api.example.com"}
	require.Equal(t, health.StatusServing, checker.CheckHealth(t.Context()))

	fb.mu.Lock()
	fb.hosts["api.example.com"] = gobreaker.StateHalfOpen
	fb.mu.Unlock()
	require.Equal(t, health.StatusDegraded, checker.CheckHealth(t.Context()))

	fb.mu.Lock()
	fb.hosts["api.example.com"] = gobreaker.StateOpen
	fb.mu.Unlock()
	require.Equal(t, health.StatusNotServing, checker.CheckHealth(t.Context()))
}

// failingClient returns a client observed by obs whose transport always fails.
func failingClient(obs httpclient.Observer, opts ...httpclient.Option) *http.Client {
	return httpclient.New(append([]httpclient.Option{
		httpclient.WithoutSSRFProtection(),
		httpclient.WithClient(&http.Client{Transport: httpx.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("forced failure")
		})}),
		httpclient.WithRetryMax(0),
		httpclient.WithBreakerTimeout(time.Hour),
		httpclient.WithObserver(obs),
	}, opts...)...)
}

func get(t *testing.T, c *http.Client, url string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := c.Do(req)
	if resp != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	require.Error(t, err)
}

func TestPushNotifyOnGlobalBreakerTrip(t *testing.T) {
	t.Parallel()
	coord := health.New()
	defer coord.Close()

	obs := New(nil, coord)
	// No per-host breakers: every host shares the client-wide breaker.
	c := failingClient(obs, httpclient.WithMaxHostBreakers(0))

	sub, err := coord.Subscribe(t.Context(), DefaultHealthServiceName)
	require.NoError(t, err)
	defer sub.Close()
	require.Equal(t, health.StatusServing, sub.InitialStatus())

	for range httpclient.DefaultBreakerMinRequests {
		get(t, c, "http://api.example.com/")
	}

	select {
	case status := <-sub.Updates():
		require.Equal(t, health.StatusNotServing, status)
	case <-time.After(2 * time.Second):
		t.Fatalf("expected push notification within 2s after breaker trip")
	}
	require.Equal(t, health.StatusNotServing, obs.health.CheckHealth(t.Context()))
}

func TestPerHostCheckerReportsBreakerState(t *testing.T) {
	t.Parallel()
	coord := health.New()
	defer coord.Close()

	obs := New(nil, coord, WithPerHostHealthChecks())
	c := failingClient(obs, httpclient.WithCircuitBreakerSettings("api.example.com", &httpclient.CircuitBreakerSettings{
		// Trip on a single failure for deterministic test behavior.
		ReadyToTrip: func(gobreaker.Counts) bool { return true },
	}))

	service := perHostServiceName(DefaultHealthServiceName, "api.example.com")
	sub, err := coord.Subscribe(t.Context(), service)
	require.NoError(t, err)
	defer sub.Close()
	require.Equal(t, health.StatusServing, sub.InitialStatus())

	get(t, c, "http://api.example.com/")

	select {
	case status := <-sub.Updates():
		require.Equal(t, health.StatusNotServing, status)
	case <-time.After(2 * time.Second):
		t.Fatalf("expected per-host push notification within 2s after breaker trip")
	}
	checker := &perHostChecker{parent: obs.health, host: "api.example.com"}
	require.Equal(t, health.StatusNotServing, checker.CheckHealth(t.Context()))
}

func TestPerHostServiceName(t *testing.T) {
	t.Parallel()
	require.Equal(t, "http_client.api.example.com",
		perHostServiceName("http_client", "api.example.com"))
}
