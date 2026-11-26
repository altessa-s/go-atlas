// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters/memory"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	prometheusadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
)

// capturingHandler records slog records at or above LevelError.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelError
}

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *capturingHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.records)
}

// OB-T1: unlabeled or wrong-label calls on labeled Prometheus instruments must not panic.
func TestPrometheus_LabeledInstrument_MismatchedLabelsDoNotPanic(t *testing.T) {
	t.Parallel()

	c, reg := newPromRegistryCollector(t)

	ctr := metrics.MustCounter(c, metrics.MetricOpts{
		Name:       "requests_total",
		Help:       "requests",
		LabelNames: []string{"method"},
	})
	g := metrics.MustGauge(c, metrics.MetricOpts{
		Name:       "inflight",
		Help:       "inflight",
		LabelNames: []string{"method"},
	})
	tm := metrics.MustTimer(c, metrics.HistogramOpts{
		Name:       "latency_seconds",
		Help:       "latency",
		LabelNames: []string{"method"},
	})

	require.NotPanics(t, func() {
		ctr.Inc()
		ctr.WithLabels(metrics.Labels{"verb": "GET"}).Inc()
		ctr.WithLabels(metrics.Labels{"method": "GET", "extra": "1"}).Inc()
	})
	require.NotPanics(t, func() {
		g.Inc()
		g.Set(5)
		g.WithLabels(metrics.Labels{"verb": "GET"}).Inc()
		g.WithLabels(metrics.Labels{"verb": "GET"}).Set(5)
	})
	require.NotPanics(t, func() {
		tm.ObserveDuration(10 * time.Millisecond)
		tm.WithLabels(metrics.Labels{"verb": "GET"}).ObserveDuration(10 * time.Millisecond)
	})

	// Valid labeled observations still record.
	ctr.WithLabels(metrics.Labels{"method": "GET"}).Inc()
	v, _ := promValue(t, reg, "svc_requests_total", map[string]string{"method": "GET"})
	require.InDelta(t, 1.0, v, 0)
}

func TestPrometheus_DroppedObservationsCounted(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	adapter := prometheusadapter.New(prometheusadapter.WithRegistry(reg))
	c := metrics.New(metrics.WithServiceName("svc"), metrics.WithAdapter(adapter))

	ctr := metrics.MustCounter(c, metrics.MetricOpts{
		Name: "requests_total", Help: "requests", LabelNames: []string{"method"},
	})
	g := metrics.MustGauge(c, metrics.MetricOpts{
		Name: "inflight", Help: "inflight", LabelNames: []string{"method"},
	})
	tm := metrics.MustTimer(c, metrics.HistogramOpts{
		Name: "latency_seconds", Help: "latency", LabelNames: []string{"method"},
	})

	ctr.Inc()
	ctr.WithLabels(metrics.Labels{"verb": "GET"}).Inc()
	g.Inc()
	g.Set(5)
	tm.ObserveDuration(time.Millisecond)

	require.Equal(t, uint64(5), adapter.DroppedObservations())
}

// OB-T2: pre-registered conflicting gauge surfaces an error on the checked path
// and logs once at Error on the recording path while returning a no-op handle.
func TestCollector_PreRegisteredConflictingGauge(t *testing.T) {
	t.Parallel()

	handler := &capturingHandler{}

	reg := prometheus.NewRegistry()
	rawGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "svc_conflict",
		Help: "pre-registered gauge",
	})
	require.NoError(t, reg.Register(rawGauge))
	rawGauge.Set(7)

	adapter := prometheusadapter.New(prometheusadapter.WithRegistry(reg))
	c := metrics.New(metrics.WithServiceName("svc"), metrics.WithAdapter(adapter), metrics.WithLogger(slog.New(handler)))

	opts := metrics.MetricOpts{Name: "conflict", Help: "conflicting counter"}

	// Checked Must* API must surface the backend registration failure.
	require.Panics(t, func() {
		metrics.MustCounter(c, opts)
	})

	// Recording API must log one Error and return a no-op handle without publishing.
	h := c.Counter(opts)
	h.Inc()
	c.Counter(opts).Inc()

	require.Equal(t, 1, handler.count(), "expected exactly one Error log on recording API")
	v, _ := promValue(t, reg, "svc_conflict", nil)
	require.InDelta(t, 7.0, v, 0)
}

// OB-T3: reusing a metric name across instrument kinds must not panic and must reject the second declaration.
func TestCollector_IncompatibleKindReuseRejectedWithoutPanic(t *testing.T) {
	t.Parallel()

	mem := memory.New()
	c := metrics.New(metrics.WithServiceName("svc"), metrics.WithAdapter(mem))

	hist := c.Histogram(metrics.HistogramOpts{Name: "duration", Help: "hist"})
	var tm metrics.Timer
	require.NotPanics(t, func() {
		tm = c.Timer(metrics.HistogramOpts{Name: "duration", Help: "hist"})
	})
	tm.ObserveDuration(2 * time.Second)
	require.Zero(t, mem.HistogramCount("svc_duration", nil), "incompatible timer handle must be a no-op")
	hist.Observe(0.5)
	require.Equal(t, uint64(1), mem.HistogramCount("svc_duration", nil))

	ctr := c.Counter(metrics.MetricOpts{Name: "items", Help: "ctr"})
	var g metrics.Gauge
	require.NotPanics(t, func() {
		g = c.Gauge(metrics.MetricOpts{Name: "items", Help: "ctr"})
	})
	g.Set(42)
	g.Inc()
	require.Zero(t, mem.GaugeValue("svc_items", nil), "incompatible gauge handle must be a no-op")
	ctr.Inc()
	require.InDelta(t, 1.0, mem.CounterValue("svc_items", nil), 0)
}

// OB-T3: reusing a metric name with different label names must be rejected on both API paths.
func TestCollector_IncompatibleLabelNamesRejected(t *testing.T) {
	t.Parallel()

	mem := memory.New()
	c := metrics.New(metrics.WithServiceName("svc"), metrics.WithAdapter(mem))

	first := c.Counter(metrics.MetricOpts{
		Name:       "requests_total",
		Help:       "requests",
		LabelNames: []string{"method"},
	})
	second := c.Counter(metrics.MetricOpts{
		Name:       "requests_total",
		Help:       "requests",
		LabelNames: []string{"route"},
	})

	second.WithLabels(metrics.Labels{"route": "/v1"}).Inc()
	require.Zero(t, mem.CounterValue("svc_requests_total", map[string]string{"route": "/v1"}),
		"incompatible label redeclaration must return a no-op handle")

	require.Panics(t, func() {
		metrics.MustCounter(c, metrics.MetricOpts{
			Name:       "requests_total",
			Help:       "requests",
			LabelNames: []string{"route"},
		})
	})

	first.WithLabels(metrics.Labels{"method": "GET"}).Inc()
	require.InDelta(t, 1.0, mem.CounterValue("svc_requests_total", map[string]string{"method": "GET"}), 0)
}

type failingLifecycleAdapter struct {
	*memory.Adapter
	flushErr   error
	closeErr   error
	flushCalls atomic.Int32
	closeCalls atomic.Int32
}

func (a *failingLifecycleAdapter) Flush() error {
	a.flushCalls.Add(1)
	return a.flushErr
}

func (a *failingLifecycleAdapter) Close() error {
	a.closeCalls.Add(1)
	return a.closeErr
}

// OB-T4: Shutdown always closes all children even when Flush or an earlier Close fails,
// joins errors, and returns the stored result on repeat calls.
func TestCollector_ShutdownAlwaysClosesAndJoinsErrors(t *testing.T) {
	t.Parallel()

	flushErr1 := errors.New("flush-1")
	closeErr1 := errors.New("close-1")
	closeErr2 := errors.New("close-2")

	a1 := &failingLifecycleAdapter{Adapter: memory.New(), flushErr: flushErr1, closeErr: closeErr1}
	a2 := &failingLifecycleAdapter{Adapter: memory.New(), closeErr: closeErr2}

	c := metrics.New(
		metrics.WithServiceName("svc"),
		metrics.WithAdapter(adapters.NewMultiAdapter(a1, a2)),
	)

	err1 := c.Shutdown(t.Context())
	require.ErrorIs(t, err1, flushErr1)
	require.ErrorIs(t, err1, closeErr1)
	require.ErrorIs(t, err1, closeErr2)
	require.Equal(t, int32(1), a1.flushCalls.Load())
	require.Equal(t, int32(1), a2.flushCalls.Load())
	require.Equal(t, int32(1), a1.closeCalls.Load())
	require.Equal(t, int32(1), a2.closeCalls.Load(), "second adapter must be closed exactly once")

	err2 := c.Shutdown(t.Context())
	require.Equal(t, err1, err2, "repeat Shutdown must return the same completed result")
	require.Equal(t, int32(1), a1.closeCalls.Load())
	require.Equal(t, int32(1), a2.closeCalls.Load())
}

// OB-T5: instruments and new registrations become no-ops after Collector.Shutdown.
func TestCollector_InertAfterShutdown(t *testing.T) {
	t.Parallel()

	t.Run("prometheus", func(t *testing.T) {
		t.Parallel()
		c, reg := newPromRegistryCollector(t)

		ctr := metrics.MustCounter(c, metrics.MetricOpts{Name: "events_total", Help: "events"})
		labeled := metrics.MustCounter(c, metrics.MetricOpts{
			Name: "labeled_total", Help: "labeled", LabelNames: []string{"k"},
		}).WithLabels(metrics.Labels{"k": "v"})
		g := metrics.MustGauge(c, metrics.MetricOpts{Name: "depth", Help: "depth"})
		tm := metrics.MustTimer(c, metrics.HistogramOpts{Name: "dur_seconds", Help: "dur"})

		ctr.Inc()
		labeled.Inc()
		g.Inc()
		tm.ObserveDuration(10 * time.Millisecond)
		stop := tm.Start()

		require.NoError(t, c.Shutdown(t.Context()))

		ctr.Inc()
		labeled.Inc()
		g.Inc()
		g.Set(99)
		stop()
		tm.ObserveDuration(10 * time.Millisecond)

		v, _ := promValue(t, reg, "svc_events_total", nil)
		require.InDelta(t, 1.0, v, 0)
		v, _ = promValue(t, reg, "svc_labeled_total", map[string]string{"k": "v"})
		require.InDelta(t, 1.0, v, 0)
		v, _ = promValue(t, reg, "svc_depth", nil)
		require.InDelta(t, 1.0, v, 0)
		_, count := promValue(t, reg, "svc_dur_seconds", nil)
		require.Equal(t, uint64(1), count)
	})

	t.Run("memory_no_post_shutdown_registration", func(t *testing.T) {
		t.Parallel()
		mem := memory.New()
		c := metrics.New(metrics.WithServiceName("svc"), metrics.WithAdapter(mem))

		ctr := metrics.MustCounter(c, metrics.MetricOpts{Name: "before_total", Help: "before"})
		ctr.Inc()
		require.NoError(t, c.Shutdown(t.Context()))

		fresh := c.Counter(metrics.MetricOpts{Name: "after_total", Help: "after"})
		fresh.Inc()
		require.False(t, mem.Exists("svc_after_total"), "registration after Shutdown must be a no-op")
	})
}

// OB-T21: WithLabels must clone the caller's map so later mutations do not affect bound handles.
func TestWithLabels_ClonesCallerMap(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	ctr := metrics.MustCounter(tc, metrics.MetricOpts{
		Name:       "requests_total",
		Help:       "requests",
		LabelNames: []string{"route"},
	})
	g := metrics.MustGauge(tc, metrics.MetricOpts{
		Name:       "inflight",
		Help:       "inflight",
		LabelNames: []string{"route"},
	})
	h := metrics.MustHistogram(tc, metrics.HistogramOpts{
		Name:       "size_bytes",
		Help:       "size",
		LabelNames: []string{"route"},
	})

	labels := metrics.Labels{"route": "A"}
	boundCtr := ctr.WithLabels(labels)
	boundGauge := g.WithLabels(labels)
	boundHist := h.WithLabels(labels)

	labels["route"] = "B"

	boundCtr.Inc()
	boundGauge.Set(3)
	boundHist.Observe(10)

	require.InDelta(t, 1.0, metricstest.GetCounterValue(t, tc, "test_requests_total", "route", "A"), 0)
	require.Zero(t, metricstest.GetCounterValue(t, tc, "test_requests_total", "route", "B"))
	require.InDelta(t, 3.0, metricstest.GetGaugeValue(t, tc, "test_inflight", "route", "A"), 0)
	require.Zero(t, metricstest.GetGaugeValue(t, tc, "test_inflight", "route", "B"))
	require.Equal(t, uint64(1), metricstest.GetHistogramCount(t, tc, "test_size_bytes", "route", "A"))
	require.Zero(t, metricstest.GetHistogramCount(t, tc, "test_size_bytes", "route", "B"))
}

// OB-T22: two collectors sharing one Prometheus registry (directly and via MultiAdapter)
// must accumulate additive gauge updates instead of overwriting with per-collector shadow values.
func TestGauge_AdditiveAcrossSharedRegistry(t *testing.T) {
	t.Parallel()

	t.Run("direct_prometheus", func(t *testing.T) {
		t.Parallel()
		reg := prometheus.NewRegistry()
		c1 := metrics.New(
			metrics.WithServiceName("svc"),
			metrics.WithAdapter(prometheusadapter.New(prometheusadapter.WithRegistry(reg))),
		)
		c2 := metrics.New(
			metrics.WithServiceName("svc"),
			metrics.WithAdapter(prometheusadapter.New(prometheusadapter.WithRegistry(reg))),
		)

		opts := metrics.MetricOpts{Name: "workers", Help: "active workers"}
		metrics.MustGauge(c1, opts).Inc()
		metrics.MustGauge(c2, opts).Inc()

		v, _ := promValue(t, reg, "svc_workers", nil)
		require.InDelta(t, 2.0, v, 0)

		labeledOpts := metrics.MetricOpts{Name: "pool_workers", Help: "pool workers", LabelNames: []string{"pool"}}
		metrics.MustGauge(c1, labeledOpts).WithLabels(metrics.Labels{"pool": "main"}).Inc()
		metrics.MustGauge(c2, labeledOpts).WithLabels(metrics.Labels{"pool": "main"}).Inc()

		v, _ = promValue(t, reg, "svc_pool_workers", map[string]string{"pool": "main"})
		require.InDelta(t, 2.0, v, 0)
	})

	t.Run("multi_adapter", func(t *testing.T) {
		t.Parallel()
		reg := prometheus.NewRegistry()
		mem1 := memory.New()
		mem2 := memory.New()

		c1 := metrics.New(
			metrics.WithServiceName("svc"),
			metrics.WithAdapter(adapters.NewMultiAdapter(
				prometheusadapter.New(prometheusadapter.WithRegistry(reg)),
				mem1,
			)),
		)
		c2 := metrics.New(
			metrics.WithServiceName("svc"),
			metrics.WithAdapter(adapters.NewMultiAdapter(
				prometheusadapter.New(prometheusadapter.WithRegistry(reg)),
				mem2,
			)),
		)

		opts := metrics.MetricOpts{Name: "workers", Help: "active workers"}
		metrics.MustGauge(c1, opts).Inc()
		metrics.MustGauge(c2, opts).Inc()

		v, _ := promValue(t, reg, "svc_workers", nil)
		require.InDelta(t, 2.0, v, 0)
		require.InDelta(t, 1.0, mem1.GaugeValue("svc_workers", nil), 0)
		require.InDelta(t, 1.0, mem2.GaugeValue("svc_workers", nil), 0)

		// Set followed by additive update keeps both GaugeAdder and shadow-Set children aligned.
		g1 := metrics.MustGauge(c1, metrics.MetricOpts{Name: "queue", Help: "queue"})
		g1.Set(10)
		g1.Add(5)
		g1.Sub(2)
		qVal, _ := promValue(t, reg, "svc_queue", nil)
		require.InDelta(t, 13.0, qVal, 0)
		require.InDelta(t, 13.0, mem1.GaugeValue("svc_queue", nil), 0)
	})
}

func TestCheckedRegistrationAPI_AndWithLogger(t *testing.T) {
	t.Parallel()

	t.Run("compatible_redeclaration_returns_same_instrument", func(t *testing.T) {
		t.Parallel()
		c := metrics.New(metrics.WithServiceName("svc"), metrics.WithAdapter(memory.New()))

		first, err := metrics.RegisterCounter(c, metrics.MetricOpts{
			Name:       "requests_total",
			Help:       "requests",
			LabelNames: []string{"b", "a"},
		})
		require.NoError(t, err)

		second, err := metrics.RegisterCounter(c, metrics.MetricOpts{
			Name:       "requests_total",
			Help:       "requests",
			LabelNames: []string{"a", "b"},
		})
		require.NoError(t, err)
		require.Same(t, first, second)
	})

	t.Run("incompatible_kind_and_labels_return_error_without_panic", func(t *testing.T) {
		t.Parallel()
		handler := &capturingHandler{}
		c := metrics.New(
			metrics.WithServiceName("svc"),
			metrics.WithAdapter(memory.New()),
			metrics.WithLogger(slog.New(handler)),
		)

		_, err := metrics.RegisterHistogram(c, metrics.HistogramOpts{Name: "dur", Help: "dur"})
		require.NoError(t, err)

		_, err = metrics.RegisterTimer(c, metrics.HistogramOpts{Name: "dur", Help: "dur"})
		require.ErrorIs(t, err, metrics.ErrDescriptorConflict)
		require.Zero(t, handler.count(), "checked API must return error without logging")

		scoped := c.WithSubsystem("http")
		_, err = metrics.RegisterCounter(scoped, metrics.MetricOpts{
			Name: "total", Help: "total", LabelNames: []string{"method"},
		})
		require.NoError(t, err)

		_, err = metrics.RegisterGauge(scoped, metrics.MetricOpts{
			Name: "total", Help: "total", LabelNames: []string{"method"},
		})
		require.ErrorIs(t, err, metrics.ErrDescriptorConflict)

		_, err = metrics.RegisterCounter(scoped, metrics.MetricOpts{
			Name: "total", Help: "total", LabelNames: []string{"route"},
		})
		require.ErrorIs(t, err, metrics.ErrDescriptorConflict)

		// Recording API logs once per rejected declaration and returns a no-op handle.
		scoped.Gauge(metrics.MetricOpts{Name: "total", Help: "total", LabelNames: []string{"method"}}).Set(5)
		scoped.Gauge(metrics.MetricOpts{Name: "total", Help: "total", LabelNames: []string{"method"}}).Set(5)
		require.Equal(t, 1, handler.count())
	})

	t.Run("backend_register_failure_checked_and_logged", func(t *testing.T) {
		t.Parallel()
		handler := &capturingHandler{}
		reg := prometheus.NewRegistry()
		require.NoError(t, reg.Register(prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "svc_taken",
			Help: "taken",
		})))

		c := metrics.New(
			metrics.WithServiceName("svc"),
			metrics.WithAdapter(prometheusadapter.New(prometheusadapter.WithRegistry(reg))),
			metrics.WithLogger(slog.New(handler)),
		)

		_, err := metrics.RegisterCounter(c, metrics.MetricOpts{Name: "taken", Help: "taken"})
		require.ErrorIs(t, err, metrics.ErrDescriptorConflict)
		require.Zero(t, handler.count())

		c.Counter(metrics.MetricOpts{Name: "taken", Help: "taken"}).Inc()
		require.Equal(t, 1, handler.count())
	})

	t.Run("checked_after_shutdown_returns_err_collector_shutdown", func(t *testing.T) {
		t.Parallel()
		c := metrics.New(metrics.WithServiceName("svc"), metrics.WithAdapter(memory.New()))
		require.NoError(t, c.Shutdown(t.Context()))

		_, err := metrics.RegisterCounter(c, metrics.MetricOpts{Name: "x", Help: "x"})
		require.ErrorIs(t, err, metrics.ErrCollectorShutdown)
	})
}
