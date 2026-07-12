// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
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
// Metric collectors come from the supplied [metrics.Collector]; the
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
}

// Dependencies returns middlewares that prometheus requires to run before it.
// Prometheus has no dependencies.
func (m *Middleware) Dependencies() []string {
	return nil
}

// New creates a new Prometheus metrics middleware with the provided options.
//
// Each call constructs a fresh middleware wired to the provided
// [metrics.Collector]. The underlying adapter deduplicates metric
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
		path := corestrings.InternLowerString(r.URL.Path)
		if m.ShouldIgnore(path) {
			m.LogIgnored(r.Context(), path)
			next.ServeHTTP(w, r)
			return
		}

		rec := newRecorder(w)
		start := time.Now()

		m.requestsInFlight.Inc()
		defer func() {
			m.requestsInFlight.Dec()
			m.recordMetrics(r, rec, start)
		}()

		next.ServeHTTP(rec, r)
	})
}

// initializeMetrics creates all metrics.
func (m *Middleware) initializeMetrics() {
	scoped := m.opts.collector.WithSubsystem(m.opts.metricsSubsystem)

	m.requestsTotal = scoped.MustCounter(metrics.MetricOpts{
		Name:       "server_requests_total",
		Help:       "Total number of HTTP server requests with method and status labels",
		LabelNames: []string{methodLabel, statusLabel},
	})

	m.requestDuration = scoped.MustHistogram(metrics.HistogramOpts{
		MetricOpts: metrics.MetricOpts{
			Name:       "server_request_duration_seconds",
			Help:       "Duration of HTTP server requests in seconds",
			LabelNames: []string{methodLabel, statusLabel},
		},
		Buckets: m.opts.durationBuckets,
	})

	m.requestsInFlight = scoped.MustGauge(metrics.MetricOpts{
		Name: "server_requests_in_flight",
		Help: "Current number of concurrent HTTP server requests being processed",
	})

	if m.opts.enableSizeMetrics {
		m.requestSize = scoped.MustHistogram(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name:       "server_request_size_bytes",
				Help:       "Size of HTTP server request bodies in bytes",
				LabelNames: []string{methodLabel, statusLabel},
			},
			Buckets: m.opts.sizeBuckets,
		})

		m.responseSize = scoped.MustHistogram(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name:       "server_response_size_bytes",
				Help:       "Size of HTTP server response bodies in bytes",
				LabelNames: []string{methodLabel, statusLabel},
			},
			Buckets: m.opts.sizeBuckets,
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
func (m *Middleware) recordMetrics(r *http.Request, rec *recorder, startTime time.Time) {
	duration := time.Since(startTime)
	statusCode := statusString(rec.StatusCode())

	internedMethod := corestrings.InternString(r.Method)

	labels := metrics.Labels{
		methodLabel: internedMethod,
		statusLabel: statusCode,
	}

	m.requestsTotal.WithLabels(labels).Inc()
	m.requestDuration.WithLabels(labels).Observe(duration.Seconds())

	if m.opts.enableSizeMetrics {
		if r.ContentLength >= 0 {
			m.requestSize.WithLabels(labels).Observe(float64(r.ContentLength))
		}
		m.responseSize.WithLabels(labels).Observe(float64(rec.Size()))
	}

	m.LogDebug(r.Context(), "recorded prometheus metrics", r.URL.Path,
		slog.String("method", r.Method),
		slog.String("status", statusCode),
		slog.Duration("duration", duration))
}
