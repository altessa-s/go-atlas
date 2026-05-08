// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/altessa-s/go-atlas/observability/health"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// httpClientHealth implements [health.Checker] over the HTTP client's
// circuit breakers and a sliding window of retry rate. It is constructed
// even when no [observability/health.Coordinator] is configured: methods
// short-circuit cheaply in that case so callers in the request hot path
// can invoke them unconditionally.
type httpClientHealth struct {
	cb          *circuitBreakerClient
	coordinator *health.Coordinator
	serviceName string
	threshold   float64
	minSamples  uint64
	perHost     bool

	// aggregate is the cross-host retry-rate window. Allocated only when a
	// coordinator is configured.
	aggregate *retrySlidingWindow
	// perHostWindows holds windows for hosts with explicit breaker settings.
	// Frozen at construction; nil when perHost is off.
	perHostWindows *coremaps.ImmutableMap[string, *retrySlidingWindow]
	// knownHosts is the set of hostnames registered as separate services.
	knownHosts map[string]struct{}
}

// retryBucket is a single slot in [retrySlidingWindow]. Bucket ownership is
// tracked by epochNanos: writers CAS the slot to claim it for the current
// bucket, then reset and increment counters.
type retryBucket struct {
	epochNanos atomic.Int64
	total      atomic.Uint64
	retried    atomic.Uint64
}

// retrySlidingWindow is a fixed-size ring of [retryBucket]s providing an
// approximate retry-rate over the configured window. Hot-path is two
// atomic increments and zero allocations.
type retrySlidingWindow struct {
	bucketDur   time.Duration
	bucketCount int
	buckets     []retryBucket
	nowFn       func() time.Time
}

func newRetrySlidingWindow(window time.Duration, bucketCount int) *retrySlidingWindow {
	if bucketCount < 1 {
		bucketCount = 1
	}
	bucketDur := window / time.Duration(bucketCount)
	if bucketDur <= 0 {
		bucketDur = time.Second
	}
	return &retrySlidingWindow{
		bucketDur:   bucketDur,
		bucketCount: bucketCount,
		buckets:     make([]retryBucket, bucketCount),
		nowFn:       time.Now,
	}
}

// record adds one observation to the current bucket; retried selects whether
// the retried counter is also incremented.
func (w *retrySlidingWindow) record(retried bool) {
	if w == nil {
		return
	}
	now := w.nowFn().UnixNano()
	bucketDur := int64(w.bucketDur)
	epoch := now - now%bucketDur
	idx := int((epoch / bucketDur) % int64(w.bucketCount))
	if idx < 0 {
		idx += w.bucketCount
	}
	b := &w.buckets[idx]

	if cur := b.epochNanos.Load(); cur != epoch {
		if b.epochNanos.CompareAndSwap(cur, epoch) {
			b.total.Store(0)
			b.retried.Store(0)
		}
	}
	b.total.Add(1)
	if retried {
		b.retried.Add(1)
	}
}

// rate returns (retried/total, total) over the active window. Buckets older
// than now-window are excluded.
func (w *retrySlidingWindow) rate(now time.Time) (float64, uint64) {
	if w == nil {
		return 0, 0
	}
	cutoff := now.UnixNano() - int64(w.bucketDur)*int64(w.bucketCount)
	var total, retried uint64
	for i := range w.buckets {
		b := &w.buckets[i]
		if b.epochNanos.Load() < cutoff {
			continue
		}
		total += b.total.Load()
		retried += b.retried.Load()
	}
	if total == 0 {
		return 0, 0
	}
	return float64(retried) / float64(total), total
}

// newHTTPClientHealth wires options into a *httpClientHealth. The returned
// value is always non-nil; methods are noops when opts.healthCoordinator
// is nil so the request hot path never has to nil-check the helper itself.
func newHTTPClientHealth(opts options, cb *circuitBreakerClient) *httpClientHealth {
	h := &httpClientHealth{
		cb:          cb,
		coordinator: opts.healthCoordinator,
		serviceName: opts.healthServiceName,
		threshold:   opts.healthRetryThreshold,
		minSamples:  opts.healthRetryMinSamples,
		perHost:     opts.healthPerHost,
	}
	if h.coordinator == nil {
		return h
	}
	h.aggregate = newRetrySlidingWindow(opts.healthRetryWindow, opts.healthRetryBuckets)
	if !h.perHost || len(opts.hostBreakerSettings) == 0 {
		return h
	}
	known := make(map[string]struct{}, len(opts.hostBreakerSettings))
	windows := make(map[string]*retrySlidingWindow, len(opts.hostBreakerSettings))
	for host := range opts.hostBreakerSettings {
		known[host] = struct{}{}
		windows[host] = newRetrySlidingWindow(opts.healthRetryWindow, opts.healthRetryBuckets)
	}
	h.knownHosts = known
	h.perHostWindows = coremaps.NewImmutableMap(windows)
	return h
}

// register attaches the configured checkers to the coordinator. Safe to
// call when the coordinator is nil — it returns immediately.
func (h *httpClientHealth) register() {
	if h == nil || h.coordinator == nil {
		return
	}
	h.coordinator.RegisterService(h.serviceName, h)
	if !h.perHost {
		return
	}
	for host := range h.knownHosts {
		h.coordinator.RegisterService(perHostServiceName(h.serviceName, host), &perHostChecker{parent: h, host: host})
	}
}

// recordRequest is invoked by the retry round-tripper for every completed
// request (after retries). It feeds the aggregate window and, when
// applicable, the per-host window.
func (h *httpClientHealth) recordRequest(host string, retried bool) {
	if h == nil || h.coordinator == nil {
		return
	}
	h.aggregate.record(retried)
	if h.perHostWindows == nil {
		return
	}
	if w, ok := h.perHostWindows.Get(host); ok {
		w.record(retried)
	}
}

// notifyAggregate recomputes the aggregate status and pushes it to the
// coordinator without waiting for the scheduler cycle. Called from the
// breaker OnStateChange chain in [circuit_breaker.go]; the recomputation
// runs in a goroutine because the gobreaker callback fires while the
// breaker's internal mutex is held — calling State() on that same breaker
// from inside the callback would self-deadlock.
func (h *httpClientHealth) notifyAggregate() {
	if h == nil || h.coordinator == nil {
		return
	}
	go func() {
		h.coordinator.NotifyStatusChange(h.serviceName, h.aggregateStatus())
	}()
}

// notifyHost notifies subscribers of a per-host service. Noop when the host
// was not registered (lazy host or perHost off). Async for the same reason
// as [notifyAggregate].
func (h *httpClientHealth) notifyHost(host string) {
	if h == nil || h.coordinator == nil || !h.perHost {
		return
	}
	if _, ok := h.knownHosts[host]; !ok {
		return
	}
	go func() {
		h.coordinator.NotifyStatusChange(perHostServiceName(h.serviceName, host), h.hostStatus(host))
	}()
}

// CheckHealth implements [health.Checker]. It aggregates breaker state and
// retry rate into a single [health.ServingStatus].
func (h *httpClientHealth) CheckHealth(_ context.Context) health.ServingStatus {
	if h == nil || h.coordinator == nil {
		return health.StatusServing
	}
	return h.aggregateStatus()
}

func (h *httpClientHealth) aggregateStatus() health.ServingStatus {
	var nClosed, nOpen, nHalfOpen int

	if h.cb != nil {
		switch h.cb.State() {
		case gobreaker.StateClosed:
			nClosed++
		case gobreaker.StateOpen:
			nOpen++
		case gobreaker.StateHalfOpen:
			nHalfOpen++
		}

		h.cb.mu.RLock()
		for _, b := range h.cb.hostBreakers {
			switch b.State() {
			case gobreaker.StateClosed:
				nClosed++
			case gobreaker.StateOpen:
				nOpen++
			case gobreaker.StateHalfOpen:
				nHalfOpen++
			}
		}
		h.cb.mu.RUnlock()
	}

	if nOpen > 0 && nClosed == 0 && nHalfOpen == 0 {
		return health.StatusNotServing
	}

	rate, samples := h.aggregate.rate(time.Now())
	retryDegraded := samples >= h.minSamples && rate > h.threshold

	if nOpen > 0 || nHalfOpen > 0 || retryDegraded {
		return health.StatusDegraded
	}
	return health.StatusServing
}

func (h *httpClientHealth) hostStatus(host string) health.ServingStatus {
	if h == nil || h.cb == nil {
		return health.StatusServing
	}
	state := gobreaker.StateClosed
	h.cb.mu.RLock()
	if breaker, exists := h.cb.hostBreakers[host]; exists {
		state = breaker.State()
	}
	h.cb.mu.RUnlock()

	var rate float64
	var samples uint64
	if h.perHostWindows != nil {
		if w, ok := h.perHostWindows.Get(host); ok {
			rate, samples = w.rate(time.Now())
		}
	}
	retryDegraded := samples >= h.minSamples && rate > h.threshold

	switch state {
	case gobreaker.StateOpen:
		return health.StatusNotServing
	case gobreaker.StateHalfOpen:
		return health.StatusDegraded
	default:
		if retryDegraded {
			return health.StatusDegraded
		}
		return health.StatusServing
	}
}

// perHostChecker adapts hostStatus to the [health.Checker] interface so the
// coordinator can poll an individual host through the standard pull model.
type perHostChecker struct {
	parent *httpClientHealth
	host   string
}

func (c *perHostChecker) CheckHealth(_ context.Context) health.ServingStatus {
	return c.parent.hostStatus(c.host)
}

// perHostServiceName builds the per-host registration key by joining the
// hostname under the base service name. The result is opaque to
// [observability/health.Coordinator] which only requires non-empty.
func perHostServiceName(base, host string) string {
	return corestrings.BuildString(func(b *strings.Builder) {
		b.WriteString(base)
		b.WriteByte('.')
		b.WriteString(host)
	})
}
