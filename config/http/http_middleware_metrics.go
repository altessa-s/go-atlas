// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"

// MetricsMiddleware defines the configuration for HTTP metrics
// collection middleware. The underlying [metrics.Collector] is wired
// separately on the ServerBuilder via `UseCollector`; this struct only
// describes per-server tunables.
type MetricsMiddleware struct {
	// BaseMiddleware provides standard enable and filtering fields.
	BaseMiddleware `yaml:",inline"`

	// Subsystem is the metric subsystem.
	Subsystem string `yaml:"subsystem"`

	// EnableSizeMetrics enables collection of request and response size metrics.
	EnableSizeMetrics bool `yaml:"enableSizeMetrics" default:"false"`

	// DurationBuckets defines the buckets for request duration histogram.
	DurationBuckets []float64 `yaml:"durationBuckets"`

	// SizeBuckets defines the buckets for request/response size histograms.
	SizeBuckets []float64 `yaml:"sizeBuckets"`
}

// Validate performs validation of the MetricsMiddleware.
func (c *MetricsMiddleware) Validate() error {
	return c.ValidateBase()
}

// DefaultMetricsMiddleware returns a configuration for the metrics
// middleware with default values.
func DefaultMetricsMiddleware() MetricsMiddleware {
	return MetricsMiddleware{
		BaseMiddleware: BaseMiddleware{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *MetricsMiddleware) IsEnabled() bool {
	return c != nil && c.Enabled
}
