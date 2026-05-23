// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

const (
	// DefaultMetricsSubsystem is the default subsystem for plugin metrics.
	DefaultMetricsSubsystem = "plugins"
)

// pluginMetrics holds all metric collectors for the plugin manager.
type pluginMetrics struct {
	// Signature verification metrics
	signatureAttempts Counter
	signatureSuccess  Counter
	signatureFailures Counter
	signatureDuration Timer

	// Plugin loading metrics
	loadAttempts  Counter
	loadSuccess   Counter
	loadFailures  Counter
	loadDuration  Timer
	pluginsLoaded Gauge

	// Quarantine metrics
	quarantineAdded   Counter
	quarantineCleared Counter
	quarantineSize    Gauge

	// Plugin state metrics
	pluginsReady      Gauge
	pluginsFailed     Gauge
	pluginsRegistered Gauge

	// Init metrics
	initAttempts Counter
	initSuccess  Counter
	initFailures Counter
	initDuration Timer
	initPanics   Counter
}

// Counter is a subset of metrics.Counter we use to avoid full interface.
type Counter interface {
	Inc()
	Add(float64)
}

// Gauge is a subset of metrics.Gauge we use to avoid full interface.
type Gauge interface {
	Set(float64)
	Inc()
	Dec()
	Add(float64)
	Sub(float64)
}

// Timer is a subset of metrics.Timer we use to avoid full interface.
type Timer interface {
	Start() (stop func())
	ObserveDuration(d time.Duration)
}

// initMetrics creates all metric collectors for the plugin manager.
func initMetrics(collector metrics.Collector) *pluginMetrics {
	if collector == nil {
		return &pluginMetrics{
			// All fields will be nil, which is safe to call methods on
			// due to nil receiver checks in the wrapper methods below
			signatureAttempts: &noopCounter{},
			signatureSuccess:  &noopCounter{},
			signatureFailures: &noopCounter{},
			signatureDuration: &noopTimer{},
			loadAttempts:      &noopCounter{},
			loadSuccess:       &noopCounter{},
			loadFailures:      &noopCounter{},
			loadDuration:      &noopTimer{},
			pluginsLoaded:     &noopGauge{},
			quarantineAdded:   &noopCounter{},
			quarantineCleared: &noopCounter{},
			quarantineSize:    &noopGauge{},
			pluginsReady:      &noopGauge{},
			pluginsFailed:     &noopGauge{},
			pluginsRegistered: &noopGauge{},
			initAttempts:      &noopCounter{},
			initSuccess:       &noopCounter{},
			initFailures:      &noopCounter{},
			initDuration:      &noopTimer{},
			initPanics:        &noopCounter{},
		}
	}

	// Create scoped collector for plugins subsystem
	c := collector.WithSubsystem(DefaultMetricsSubsystem)

	return &pluginMetrics{
		// Signature verification metrics
		signatureAttempts: c.Counter(metrics.MetricOpts{
			Name: "signature_verification_attempts_total",
			Help: "Total number of plugin signature verification attempts",
		}),
		signatureSuccess: c.Counter(metrics.MetricOpts{
			Name: "signature_verification_success_total",
			Help: "Total number of successful plugin signature verifications",
		}),
		signatureFailures: c.Counter(metrics.MetricOpts{
			Name: "signature_verification_failures_total",
			Help: "Total number of failed plugin signature verifications",
		}),
		signatureDuration: c.Timer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "signature_verification_duration_seconds",
				Help: "Plugin signature verification duration in seconds",
			},
		}),

		// Plugin loading metrics
		loadAttempts: c.Counter(metrics.MetricOpts{
			Name: "load_attempts_total",
			Help: "Total number of plugin load attempts",
		}),
		loadSuccess: c.Counter(metrics.MetricOpts{
			Name: "load_success_total",
			Help: "Total number of successful plugin loads",
		}),
		loadFailures: c.Counter(metrics.MetricOpts{
			Name: "load_failures_total",
			Help: "Total number of failed plugin loads",
		}),
		loadDuration: c.Timer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "load_duration_seconds",
				Help: "Plugin load duration in seconds",
			},
		}),
		pluginsLoaded: c.Gauge(metrics.MetricOpts{
			Name: "loaded_total",
			Help: "Current number of loaded plugins",
		}),

		// Quarantine metrics
		quarantineAdded: c.Counter(metrics.MetricOpts{
			Name: "quarantine_added_total",
			Help: "Total number of plugins added to quarantine",
		}),
		quarantineCleared: c.Counter(metrics.MetricOpts{
			Name: "quarantine_cleared_total",
			Help: "Total number of plugins cleared from quarantine",
		}),
		quarantineSize: c.Gauge(metrics.MetricOpts{
			Name: "quarantine_size",
			Help: "Current number of quarantined plugins",
		}),

		// Plugin state metrics
		pluginsReady: c.Gauge(metrics.MetricOpts{
			Name: "ready",
			Help: "Current number of plugins in ready state",
		}),
		pluginsFailed: c.Gauge(metrics.MetricOpts{
			Name: "failed",
			Help: "Current number of plugins in failed state",
		}),
		pluginsRegistered: c.Gauge(metrics.MetricOpts{
			Name: "registered",
			Help: "Current number of registered plugins",
		}),

		// Init metrics
		initAttempts: c.Counter(metrics.MetricOpts{
			Name: "init_attempts_total",
			Help: "Total number of plugin Init calls",
		}),
		initSuccess: c.Counter(metrics.MetricOpts{
			Name: "init_success_total",
			Help: "Total number of successful plugin Init calls",
		}),
		initFailures: c.Counter(metrics.MetricOpts{
			Name: "init_failures_total",
			Help: "Total number of failed plugin Init calls",
		}),
		initDuration: c.Timer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "init_duration_seconds",
				Help: "Plugin Init duration in seconds",
			},
		}),
		initPanics: c.Counter(metrics.MetricOpts{
			Name: "init_panics_total",
			Help: "Total number of panics during plugin Init",
		}),
	}
}

// noopCounter is a no-op Counter implementation.
type noopCounter struct{}

func (n *noopCounter) Inc()          {}
func (n *noopCounter) Add(_ float64) {}

// noopGauge is a no-op Gauge implementation.
type noopGauge struct{}

func (n *noopGauge) Set(_ float64) {}
func (n *noopGauge) Inc()          {}
func (n *noopGauge) Dec()          {}
func (n *noopGauge) Add(_ float64) {}
func (n *noopGauge) Sub(_ float64) {}

// noopTimer is a no-op Timer implementation.
type noopTimer struct{}

func (n *noopTimer) Start() func()                   { return func() {} }
func (n *noopTimer) ObserveDuration(d time.Duration) {}

// updateStateMetrics updates gauge metrics based on current plugin states.
func (m *Manager) updateStateMetrics() {
	if m.metrics == nil {
		return
	}

	var ready, failed, registered int
	m.mu.RLock()
	registered = len(m.plugins)
	for _, p := range m.plugins {
		switch p.State() {
		case StateReady:
			ready++
		case StateFailed:
			failed++
		case StateLoaded, StateUnloaded:
			// StateLoaded: plugins not yet initialized
			// StateUnloaded: plugins that have been removed
			// These are not counted as ready or failed
		}
	}
	m.mu.RUnlock()

	m.metrics.pluginsReady.Set(float64(ready))
	m.metrics.pluginsFailed.Set(float64(failed))
	m.metrics.pluginsRegistered.Set(float64(registered))

	// Update quarantine size
	m.quarantineMu.RLock()
	quarantineSize := len(m.quarantine)
	m.quarantineMu.RUnlock()
	m.metrics.quarantineSize.Set(float64(quarantineSize))
}
