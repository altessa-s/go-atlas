// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/router"
)

const middlewareName = "metrics"

// Name returns the middleware name used for dependency resolution and chain ordering.
func Name() string { return middlewareName }

// ID is a lightweight [middlewares.Middleware] reference for this package,
// suitable for passing to exclusion lists.
var ID = middlewares.Noop(middlewareName)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*Middleware)(nil)

// Middleware provides Prometheus metrics collection for HTTP servers.
// Metric collectors come from the supplied [metrics.Meter]; the
// underlying adapter deduplicates registrations by name, so calling [New]
// multiple times against the same collector is safe and shares the same
// metric vectors. To emit metrics for two unrelated upstreams against a
// single registry, pass distinct subsystems via [WithMetricsSubsystem].
type Middleware struct {
	middlewares.BaseMiddleware
	opts             *options
	requestsTotal    metrics.Counter
	requestDuration  metrics.Histogram
	requestsInFlight metrics.Gauge
	requestSize      metrics.Histogram
	responseSize     metrics.Histogram
	// handles caches bound instruments per combination of method, status and
	// registered route template.
	handles sync.Map // requestLabels → *requestHandles
}

type requestLabels struct {
	method string
	status string
	route  string
}

// requestHandles caches the bound handles for one (method, status) pair.
type requestHandles struct {
	total    metrics.Counter
	duration metrics.Histogram
	reqSize  metrics.Histogram // nil unless size metrics enabled
	respSize metrics.Histogram
}

// handlesFor returns the cached instruments, binding the labels on first use.
func (m *Middleware) handlesFor(key requestLabels) *requestHandles {
	if v, ok := m.handles.Load(key); ok {
		return v.(*requestHandles) //nolint:errcheck // only *requestHandles is stored
	}

	labels := metrics.Labels{methodLabel: key.method, statusLabel: key.status, routeLabel: key.route}
	h := &requestHandles{
		total:    m.requestsTotal.WithLabels(labels),
		duration: m.requestDuration.WithLabels(labels),
	}
	if m.requestSize != nil {
		h.reqSize = m.requestSize.WithLabels(labels)
	}
	if m.responseSize != nil {
		h.respSize = m.responseSize.WithLabels(labels)
	}
	v, _ := m.handles.LoadOrStore(key, h)
	return v.(*requestHandles) //nolint:errcheck // only *requestHandles is stored
}

// Dependencies returns middlewares that prometheus requires to run before it.
// Prometheus has no dependencies.
func (m *Middleware) Dependencies() []string {
	return nil
}

// New creates a new Prometheus metrics middleware with the provided options.
//
// Each call constructs a fresh middleware wired to the provided
// [metrics.Meter]. The underlying adapter deduplicates metric
// registrations by name, so reusing the same collector across multiple
// calls with the same subsystem is safe and shares the same metric
// vectors.
func New(opt ...Option) *Middleware {
	opts := newOptions(opt...)

	m := &Middleware{
		BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter(
			middlewareName,
			opts.ignorePaths,
			opts.ignorePatterns,
			opts.logger,
		),
		opts: opts,
	}
	m.initializeMetrics()
	return m
}

// Handler wraps an http.Handler with Prometheus metrics collection.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.ShouldIgnoreRequest(r) {
			m.LogIgnored(r.Context(), router.RouteTemplate(r))
			next.ServeHTTP(w, r)
			return
		}

		rec := middlewares.NewResponseWriter(w, false)
		defer rec.Release()
		start := time.Now()
		route := router.RouteTemplate(r)

		m.requestsInFlight.Inc()
		defer func() {
			m.requestsInFlight.Dec()
			m.recordMetrics(r, rec, start, route)
		}()

		next.ServeHTTP(rec, r)
	})
}

// initializeMetrics creates all metrics.
func (m *Middleware) initializeMetrics() {
	scoped := metrics.MustMeter{Meter: m.opts.meter.WithSubsystem(m.opts.metricsSubsystem)}

	m.requestsTotal = scoped.MustCounter(metrics.MetricOpts{
		Name:       "server_requests_total",
		Help:       "Total number of HTTP server requests with method, status and route labels",
		LabelNames: []string{methodLabel, statusLabel, routeLabel},
	})

	m.requestDuration = scoped.MustHistogram(metrics.HistogramOpts{
		Name:       "server_request_duration_seconds",
		Help:       "Duration of HTTP server requests in seconds",
		LabelNames: []string{methodLabel, statusLabel, routeLabel},
		Buckets:    m.opts.durationBuckets,
	})

	m.requestsInFlight = scoped.MustGauge(metrics.MetricOpts{
		Name: "server_requests_in_flight",
		Help: "Current number of concurrent HTTP server requests being processed",
	})

	if m.opts.enableSizeMetrics {
		m.requestSize = scoped.MustHistogram(metrics.HistogramOpts{
			Name:       "server_request_size_bytes",
			Help:       "Size of HTTP server request bodies in bytes",
			LabelNames: []string{methodLabel, statusLabel, routeLabel},
			Buckets:    m.opts.sizeBuckets,
		})

		m.responseSize = scoped.MustHistogram(metrics.HistogramOpts{
			Name:       "server_response_size_bytes",
			Help:       "Size of HTTP server response bodies in bytes",
			LabelNames: []string{methodLabel, statusLabel, routeLabel},
			Buckets:    m.opts.sizeBuckets,
		})
	}
}

// statusStrings caches the decimal form of every valid HTTP status code so
// the per-request path avoids strconv.Itoa and an interner lookup.
var statusStrings = func() [600]string {
	var a [600]string
	for i := 100; i < 600; i++ {
		a[i] = strconv.Itoa(i)
	}
	return a
}()

// statusString returns the decimal form of an HTTP status code, falling back
// to strconv for out-of-range values.
func statusString(code int) string {
	if code >= 100 && code < 600 {
		return statusStrings[code]
	}
	return strconv.Itoa(code)
}

// recordMetrics records all metrics for a completed request.
func (m *Middleware) recordMetrics(r *http.Request, rec *middlewares.ResponseWriter, startTime time.Time, route string) {
	duration := time.Since(startTime)
	statusCode := statusString(rec.StatusCode())

	h := m.handlesFor(requestLabels{method: middlewares.BoundedMethod(r.Method), status: statusCode, route: route})

	h.total.Inc()
	h.duration.Observe(duration.Seconds())

	if m.opts.enableSizeMetrics {
		if r.ContentLength >= 0 {
			h.reqSize.Observe(float64(r.ContentLength))
		}
		h.respSize.Observe(float64(rec.BytesWritten()))
	}

	m.LogDebug(r.Context(), "recorded prometheus metrics", route,
		slog.String("method", middlewares.BoundedMethod(r.Method)),
		slog.String("status", statusCode),
		slog.Duration("duration", duration))
}
