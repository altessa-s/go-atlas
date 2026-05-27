// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
)

// newTestHealth constructs a *httpClientHealth attached to a fresh
// *circuitBreakerClient. When coord is non-nil the helper also calls
// register() so callers can validate Coordinator-side registration.
func newTestHealth(t *testing.T, coord *health.Coordinator, optMutators ...Option) *httpClientHealth {
	t.Helper()
	opts := newOptions(append([]Option{
		WithHealthRetryWindow(60 * time.Second),
		WithHealthRetryThreshold(0.2),
		WithHealthRetryMinSamples(10),
	}, optMutators...)...)
	if coord != nil {
		opts.healthCoordinator = coord
	}
	m := newHTTPClientMetrics(nil, "")
	cb := newCircuitBreakerClient(*opts, m)
	h := newHTTPClientHealth(*opts, cb)
	cb.health.Store(h)
	h.register()
	return h
}

// tripGlobalBreaker drives the global circuit breaker into the open state by
// repeatedly executing failing operations. It uses defaultReadyToTrip
// thresholds (≥ DefaultBreakerMinRequests with 100% failure ratio).
func tripGlobalBreaker(t *testing.T, cb *circuitBreakerClient) {
	t.Helper()
	forcedErr := errors.New("forced failure for breaker trip")
	for range DefaultBreakerMinRequests {
		_, err := cb.CircuitBreaker.Execute(func() (*http.Response, error) {
			return nil, forcedErr
		})
		require.Error(t, err)
	}
	require.Equal(t, gobreaker.StateOpen, cb.CircuitBreaker.State())
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

func TestNewHTTPClientHealthNoCoordinator(t *testing.T) {
	t.Parallel()
	opts := newOptions()
	h := newHTTPClientHealth(*opts, nil)
	require.NotNil(t, h)
	require.Nil(t, h.coordinator)
	require.Nil(t, h.aggregate, "aggregate window must not be allocated when coordinator is absent")
	require.Nil(t, h.perHostWindows)

	// Method calls must be cheap noops.
	h.recordRequest("api.example.com", true)
	h.notifyAggregate()
	h.notifyHost("api.example.com")
	require.Equal(t, health.StatusServing, h.CheckHealth(t.Context()))
}

func TestNewHTTPClientHealthPerHostFiltersToConfiguredHosts(t *testing.T) {
	t.Parallel()
	coord := health.New()
	defer coord.Close()

	settings := &CircuitBreakerSettings{Name: "cb"}
	h := newTestHealth(t, coord,
		WithCircuitBreakerSettings("api.example.com", settings),
		WithCircuitBreakerSettings("backup.example.com", settings),
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

func TestNewHTTPClientHealthPerHostOffSkipsRegistration(t *testing.T) {
	t.Parallel()
	coord := health.New()
	defer coord.Close()

	h := newTestHealth(t, coord,
		WithCircuitBreakerSettings("api.example.com", &CircuitBreakerSettings{Name: "cb"}),
	)

	require.Nil(t, h.perHostWindows)
	require.Equal(t, []string{"http_client"}, slices.Collect(coord.ListServices()))
}

func TestRecordRequestUpdatesPerHostWindow(t *testing.T) {
	t.Parallel()
	coord := health.New()
	defer coord.Close()

	h := newTestHealth(t, coord,
		WithCircuitBreakerSettings("api.example.com", &CircuitBreakerSettings{Name: "cb"}),
		WithPerHostHealthChecks(),
	)

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

	h := newTestHealth(t, coord)
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

	h := newTestHealth(t, coord)
	// 12 retried / 8 plain = 60% retry rate, 20 samples.
	for range 12 {
		h.recordRequest("api.example.com", true)
	}
	for range 8 {
		h.recordRequest("api.example.com", false)
	}
	require.Equal(t, health.StatusDegraded, h.CheckHealth(t.Context()))
}

func TestAggregateStatusFreshClientServing(t *testing.T) {
	t.Parallel()
	coord := health.New()
	defer coord.Close()

	h := newTestHealth(t, coord)
	require.Equal(t, health.StatusServing, h.CheckHealth(t.Context()))
}

func TestAggregateStatusGlobalBreakerOpenIsNotServing(t *testing.T) {
	t.Parallel()
	coord := health.New()
	defer coord.Close()

	h := newTestHealth(t, coord)
	tripGlobalBreaker(t, h.cb)

	// Global breaker is the only breaker that exists → all-open ⇒ NotServing.
	require.Equal(t, health.StatusNotServing, h.CheckHealth(t.Context()))
}

func TestPushNotifyOnGlobalBreakerTrip(t *testing.T) {
	t.Parallel()
	coord := health.New()
	defer coord.Close()

	h := newTestHealth(t, coord)

	sub, err := coord.Subscribe(t.Context(), h.serviceName)
	require.NoError(t, err)
	defer sub.Close()
	require.Equal(t, health.StatusServing, sub.InitialStatus())

	tripGlobalBreaker(t, h.cb)

	select {
	case status := <-sub.Updates():
		require.Equal(t, health.StatusNotServing, status)
	case <-time.After(2 * time.Second):
		t.Fatalf("expected push notification within 2s after breaker trip")
	}
}

func TestPerHostCheckerReportsBreakerState(t *testing.T) {
	t.Parallel()
	coord := health.New()
	defer coord.Close()

	settings := &CircuitBreakerSettings{
		// Trip on a single failure for deterministic test behavior.
		ReadyToTrip: func(_ gobreaker.Counts) bool { return true },
	}
	h := newTestHealth(t, coord,
		WithCircuitBreakerSettings("api.example.com", settings),
		WithPerHostHealthChecks(),
	)

	checker := &perHostChecker{parent: h, host: "api.example.com"}
	require.Equal(t, health.StatusServing, checker.CheckHealth(t.Context()))

	// Force the per-host breaker to materialize and trip it.
	hostCB := h.cb.getBreakerForHost("api.example.com")
	_, err := hostCB.Execute(func() (*http.Response, error) {
		return nil, errors.New("forced")
	})
	require.Error(t, err)
	require.Equal(t, gobreaker.StateOpen, hostCB.State())
	require.Equal(t, health.StatusNotServing, checker.CheckHealth(t.Context()))
}

func TestPerHostServiceName(t *testing.T) {
	t.Parallel()
	require.Equal(t, "http_client.api.example.com",
		perHostServiceName("http_client", "api.example.com"))
}
