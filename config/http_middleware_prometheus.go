// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

// HttpInterPrometheusConfig defines the configuration for HTTP Prometheus metrics collection middleware.
type HttpInterPrometheusConfig struct {
	// BaseHttpMiddlewareConfig provides standard enable and filtering fields.
	BaseHttpMiddlewareConfig `yaml:",inline"`

	// Namespace is the Prometheus metric namespace.
	Namespace string `yaml:"namespace"`

	// Subsystem is the Prometheus metric subsystem.
	Subsystem string `yaml:"subsystem"`

	// EnableSizeMetrics enables collection of request and response size metrics.
	EnableSizeMetrics bool `yaml:"enableSizeMetrics" default:"false"`

	// DurationBuckets defines the buckets for request duration histogram.
	DurationBuckets []float64 `yaml:"durationBuckets"`

	// SizeBuckets defines the buckets for request/response size histograms.
	SizeBuckets []float64 `yaml:"sizeBuckets"`
}

// Validate performs validation of the HttpInterPrometheusConfig.
func (c *HttpInterPrometheusConfig) Validate() error {
	return c.ValidateBase()
}

// DefaultHttpInterPrometheusConfig returns a configuration for prometheus middleware with default values.
func DefaultHttpInterPrometheusConfig() HttpInterPrometheusConfig {
	return HttpInterPrometheusConfig{
		BaseHttpMiddlewareConfig: BaseHttpMiddlewareConfig{
			EnableMixin: EnableMixin{Enable: false},
		},
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *HttpInterPrometheusConfig) IsEnabled() bool {
	return c != nil && c.Enable
}
