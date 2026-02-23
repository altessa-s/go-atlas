// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
	prominternal "github.com/altessa-s/go-atlas/transport/internal/prometheus"
)

// Prometheus metric labels (use shared interned labels)
var (
	methodLabel = prominternal.MethodLabel
	statusLabel = prominternal.StatusLabel
)

// Metric name suffixes
const (
	metricServerRequestsTotal          = "server_requests_total"
	metricServerRequestDurationSeconds = "server_request_duration_seconds"
	metricServerRequestsInFlight       = "server_requests_in_flight"
	metricServerRequestSizeBytes       = "server_request_size_bytes"
	metricServerResponseSizeBytes      = "server_response_size_bytes"
)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*Middleware)(nil)

// Middleware provides Prometheus metrics collection for HTTP servers.
// Metric collectors are shared via a process-wide singleton (see
// [getOrCreateServerMetrics]), so calling [New] multiple times is safe and
// avoids "already registered" errors. Each instance may have its own path
// filter configuration. The Handler method is safe for concurrent use.
type Middleware struct {
	middlewares.BaseMiddleware
	opts              *options
	requestsTotal     *prometheus.CounterVec
	requestDuration   *prometheus.HistogramVec
	requestsInFlight  prometheus.Gauge
	requestSize       *prometheus.HistogramVec
	responseSize      *prometheus.HistogramVec
	registeredMetrics []prometheus.Collector
}

// Dependencies returns middlewares that prometheus requires to run before it.
// Prometheus has no dependencies.
func (m *Middleware) Dependencies() []string {
	return nil
}

// New creates a new Prometheus metrics middleware with the provided options.
//
// This function uses a singleton pattern to ensure that Prometheus metrics are registered
// only once per process. Calling this function multiple times will return middlewares
// that share the same underlying metrics collectors.
//
// Note: The metrics configuration (namespace, subsystem, buckets) is determined by
// the first call to this function. Subsequent calls with different options will use
// the metrics from the first initialization, but can have their own ignore patterns.
func New(opt ...Option) *Middleware {
	opts := newOptions(opt...)

	// Use singleton pattern to avoid "already registered" errors
	m := getOrCreateServerMetrics(opts)

	// Return a wrapper that uses the shared metrics but has its own ignore checker
	return &Middleware{
		BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter(
			"prometheus",
			opts.ignorePaths,
			opts.ignorePatterns,
			opts.logger,
		),
		opts:              m.opts,
		requestsTotal:     m.requestsTotal,
		requestDuration:   m.requestDuration,
		requestsInFlight:  m.requestsInFlight,
		requestSize:       m.requestSize,
		responseSize:      m.responseSize,
		registeredMetrics: m.registeredMetrics,
	}
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

// initializeMetrics creates all Prometheus metrics collectors.
func (m *Middleware) initializeMetrics() {
	// Core request metrics
	m.requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: m.buildMetricName(metricServerRequestsTotal),
			Help: "Total number of HTTP server requests with method and status labels",
		}, []string{methodLabel, statusLabel})

	m.requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    m.buildMetricName(metricServerRequestDurationSeconds),
			Help:    "Duration of HTTP server requests in seconds",
			Buckets: m.opts.durationBuckets,
		}, []string{methodLabel, statusLabel})

	m.requestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: m.buildMetricName(metricServerRequestsInFlight),
			Help: "Current number of concurrent HTTP server requests being processed",
		})

	// Optional size metrics
	if m.opts.enableSizeMetrics {
		m.requestSize = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    m.buildMetricName(metricServerRequestSizeBytes),
				Help:    "Size of HTTP server request bodies in bytes",
				Buckets: m.opts.sizeBuckets,
			}, []string{methodLabel, statusLabel})

		m.responseSize = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    m.buildMetricName(metricServerResponseSizeBytes),
				Help:    "Size of HTTP server response bodies in bytes",
				Buckets: m.opts.sizeBuckets,
			}, []string{methodLabel, statusLabel})
	}

	m.LogDebug(context.Background(), "initialized prometheus metrics", "",
		slog.String("namespace", m.opts.namespace),
		slog.String("subsystem", m.opts.subsystem),
		slog.Bool("size_metrics", m.opts.enableSizeMetrics))
}

// registerMetrics registers all metrics with the configured registerer.
func (m *Middleware) registerMetrics() {
	collectors := []prometheus.Collector{
		m.requestsTotal,
		m.requestDuration,
		m.requestsInFlight,
	}

	if m.opts.enableSizeMetrics {
		collectors = append(collectors, m.requestSize, m.responseSize)
	}

	for _, collector := range collectors {
		if err := m.opts.registerer.Register(collector); err != nil {
			if _, ok := coreerrs.AsType[prometheus.AlreadyRegisteredError](err); ok { //nolint:errcheck // only checking ok
				m.LogDebug(context.Background(), "metric already registered, using existing one", "",
					slog.Any("error", err))
			}
		}
	}

	m.registeredMetrics = collectors
	m.LogDebug(context.Background(), "registered prometheus metrics", "",
		slog.Int("count", len(collectors)))
}

// recordMetrics records all metrics for a completed request.
func (m *Middleware) recordMetrics(r *http.Request, rec *recorder, startTime time.Time) {
	duration := time.Since(startTime)
	statusCode := strconv.Itoa(rec.StatusCode())

	// Intern strings to reduce memory usage for Prometheus labels
	internedMethod := corestrings.InternString(r.Method)
	internedStatus := corestrings.InternString(statusCode)

	// Record core metrics
	m.requestsTotal.WithLabelValues(internedMethod, internedStatus).Inc()
	m.requestDuration.WithLabelValues(internedMethod, internedStatus).Observe(duration.Seconds())

	// Record optional size metrics
	if m.opts.enableSizeMetrics {
		if r.ContentLength >= 0 {
			m.requestSize.WithLabelValues(internedMethod, internedStatus).Observe(float64(r.ContentLength))
		}
		m.responseSize.WithLabelValues(internedMethod, internedStatus).Observe(float64(rec.Size()))
	}

	m.LogDebug(r.Context(), "recorded prometheus metrics", r.URL.Path,
		slog.String("method", r.Method),
		slog.String("status", statusCode),
		slog.Duration("duration", duration))
}

// buildMetricName builds a metric name with namespace and subsystem prefixes.
func (m *Middleware) buildMetricName(suffix string) string {
	return prominternal.BuildMetricNameWithDefault(m.opts.namespace, m.opts.subsystem, suffix, DefaultMetricPrefix)
}
