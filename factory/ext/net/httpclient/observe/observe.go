// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observe

import (
	"cmp"
	"sync"

	"github.com/sony/gobreaker/v2"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"

	httpclient "github.com/altessa-s/go-atlas-net/http/client"
)

// Observer is an [httpclient.Observer] that records the client's events as
// metrics and, with a health reporter, derives health checks from them.
// Create one per client with [New] and pass it to [httpclient.WithObserver].
type Observer struct {
	metrics *clientMetrics
	health  *clientHealth
}

var _ httpclient.Observer = (*Observer)(nil)

// New returns an [Observer] that registers the client metrics on meter
// (under [WithMetricsSubsystem]) and, when reporter is non-nil, registers an
// aggregate health checker (and, with [WithPerHostHealthChecks], one per
// configured host) with it once the observer is attached to a client. A nil
// meter records no metrics; a nil reporter disables the health integration.
func New(meter metrics.Meter, reporter health.Reporter, opts ...Option) *Observer {
	o := newOptions(opts...)
	return &Observer{
		metrics: newClientMetrics(meter, o.metricsSubsystem),
		health:  newClientHealth(*o, reporter),
	}
}

// Attach implements [httpclient.Observer]: it registers the health checkers.
func (o *Observer) Attach(breakers httpclient.Breakers) {
	o.health.attach(breakers)
}

// RequestDone implements [httpclient.Observer].
func (o *Observer) RequestDone(e httpclient.RequestEvent) {
	o.metrics.durationFor(e.Method).ObserveDuration(e.Duration)
	o.metrics.totalFor(e.Method, e.StatusClass).Inc()
	if e.Failed {
		o.metrics.errorsFor(e.Method).Inc()
	}
	o.health.recordRequest(e.Host, e.Retries > 0)
}

// Retry implements [httpclient.Observer].
func (o *Observer) Retry(httpclient.AttemptEvent) {
	o.metrics.retries.Inc()
}

// BreakerRejected implements [httpclient.Observer].
func (o *Observer) BreakerRejected(httpclient.AttemptEvent) {
	o.metrics.circuitBreakerTrips.Inc()
}

// BreakerStateChanged implements [httpclient.Observer]. The breaker-state
// gauge is labeled with the host of a per-host breaker and with the name of
// the client-wide one.
func (o *Observer) BreakerStateChanged(e httpclient.BreakerStateEvent) {
	o.metrics.circuitBreakerState.WithLabels(metrics.Labels{"host": cmp.Or(e.Host, e.Name)}).Set(stateToFloat(e.To))
	o.health.notifyAggregate()
	if e.Host != "" {
		o.health.notifyHost(e.Host)
	}
}

// stateToFloat converts a gobreaker state to a float64 for gauge metrics.
// 0=closed, 1=half-open, 2=open.
func stateToFloat(s gobreaker.State) float64 {
	//nolint:mnd
	switch s {
	case gobreaker.StateClosed:
		return 0
	case gobreaker.StateHalfOpen:
		return 1
	case gobreaker.StateOpen:
		return 2
	default:
		return 0
	}
}

// labelMethod is the request-method label of the client metrics.
const labelMethod = "method"

// clientMetrics holds all metrics of the HTTP client. When no
// [metrics.Meter] is provided, [metrics.Noop] is used and all methods become
// zero-cost no-ops.
type clientMetrics struct {
	requestsTotal       metrics.Counter
	requestErrors       metrics.Counter
	retries             metrics.Counter
	requestDuration     metrics.Timer
	circuitBreakerTrips metrics.Counter
	circuitBreakerState metrics.Gauge

	// Per-request label sets are cached so the hot request path does not
	// rebuild label maps per call. Cardinality is bounded by HTTP methods
	// (× 5 status classes for totals).
	durations sync.Map // method → metrics.Timer
	totals    sync.Map // totalKey → metrics.Counter
	errors    sync.Map // method → metrics.Counter
}

// durationFor returns the requestDuration timer bound to the method label,
// binding it on first use.
func (m *clientMetrics) durationFor(method string) metrics.Timer {
	if v, ok := m.durations.Load(method); ok {
		return v.(metrics.Timer) //nolint:errcheck
	}
	v, _ := m.durations.LoadOrStore(method, m.requestDuration.WithLabels(metrics.Labels{labelMethod: method}))
	return v.(metrics.Timer) //nolint:errcheck
}

// totalKey keys the totals cache; a comparable struct avoids building a
// concatenated string key on every lookup.
type totalKey struct{ method, statusClass string }

// totalFor returns the requestsTotal counter bound to the (method,
// status_class) label pair, binding it on first use.
func (m *clientMetrics) totalFor(method, statusClass string) metrics.Counter {
	key := totalKey{method: method, statusClass: statusClass}
	if v, ok := m.totals.Load(key); ok {
		return v.(metrics.Counter) //nolint:errcheck
	}
	v, _ := m.totals.LoadOrStore(key, m.requestsTotal.WithLabels(metrics.Labels{labelMethod: method, "status_class": statusClass}))
	return v.(metrics.Counter) //nolint:errcheck
}

// errorsFor returns the requestErrors counter bound to the method label,
// binding it on first use.
func (m *clientMetrics) errorsFor(method string) metrics.Counter {
	if v, ok := m.errors.Load(method); ok {
		return v.(metrics.Counter) //nolint:errcheck
	}
	v, _ := m.errors.LoadOrStore(method, m.requestErrors.WithLabels(metrics.Labels{labelMethod: method}))
	return v.(metrics.Counter) //nolint:errcheck
}

func newClientMetrics(c metrics.Meter, subsystem string) *clientMetrics {
	if c == nil {
		c = metrics.Noop()
	}
	if subsystem == "" {
		subsystem = DefaultMetricsSubsystem
	}

	scoped := c.WithSubsystem(subsystem)

	return &clientMetrics{
		requestsTotal: scoped.MustCounter(metrics.MetricOpts{
			Name:       "requests_total",
			Help:       "Total number of HTTP requests completed.",
			LabelNames: []string{labelMethod, "status_class"},
		}),
		requestErrors: scoped.MustCounter(metrics.MetricOpts{
			Name:       "request_errors_total",
			Help:       "Total number of HTTP request errors after all retries.",
			LabelNames: []string{labelMethod},
		}),
		retries: scoped.MustCounter(metrics.MetricOpts{
			Name: "retries_total",
			Help: "Total number of HTTP request retry attempts.",
		}),
		requestDuration: scoped.MustTimer(metrics.HistogramOpts{
			Name:       "request_duration_seconds",
			Help:       "Duration of HTTP requests including retries in seconds.",
			LabelNames: []string{labelMethod},
		}),
		circuitBreakerTrips: scoped.MustCounter(metrics.MetricOpts{
			Name: "circuit_breaker_trips_total",
			Help: "Total number of circuit breaker trip events.",
		}),
		circuitBreakerState: scoped.MustGauge(metrics.MetricOpts{
			Name:       "circuit_breaker_state",
			Help:       "Current state of the circuit breaker per host (0=closed, 1=half-open, 2=open).",
			LabelNames: []string{"host"},
		}),
	}
}
