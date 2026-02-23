// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import validation "github.com/go-ozzo/ozzo-validation/v4"

// Default values for Metrics configuration.
const (
	defaultMetricsEnable = false
	defaultMetricsType   = MetricsTypePrometheus
)

// MetricsType represents the type of metrics backend.
type MetricsType string

// Supported metrics types.
const (
	// MetricsTypePrometheus uses Prometheus for metrics collection.
	MetricsTypePrometheus MetricsType = "prometheus"

	// MetricsTypeNoop disables metrics collection.
	MetricsTypeNoop MetricsType = "noop"
)

// Metrics represents the configuration for the metrics system.
//
// Example:
//
//	metrics:
//	  enable: true
//	  type: prometheus
//	  serviceName: myapp
//	  adapters:
//	    prometheus:
//	      customRegistry: false
type Metrics struct {
	// Enable determines whether metrics collection is enabled.
	// Defaults to false.
	Enable bool `yaml:"enable" default:"false"`

	// Type specifies the metrics backend type.
	// Valid values: "prometheus", "noop".
	// Defaults to "prometheus".
	Type MetricsType `yaml:"type" default:"prometheus"`

	// ServiceName is the global prefix for all metrics.
	// This becomes the first part of the metric name: {serviceName}_{subsystem}_{name}.
	// Example: "myapp" results in metrics like "myapp_http_requests_total".
	ServiceName string `yaml:"serviceName"`

	// Adapters contains adapter-specific configurations.
	Adapters *MetricsAdapters `yaml:"adapters,omitempty" default:"-"`
}

// MetricsAdapters contains configurations for different metrics adapters.
type MetricsAdapters struct {
	// Prometheus contains Prometheus-specific configuration.
	// Only used when Type is "prometheus".
	Prometheus *MetricsPrometheus `yaml:"prometheus,omitempty" default:"-"`
}

// DefaultMetrics returns a Metrics configuration with default values.
func DefaultMetrics() Metrics {
	return Metrics{
		Enable: defaultMetricsEnable,
		Type:   defaultMetricsType,
	}
}

// Validate performs validation on the Metrics configuration.
func (m Metrics) Validate() error {
	if !m.Enable {
		return nil
	}

	return ValidateStruct(&m,
		validation.Field(&m.Type,
			validation.Required,
			validation.In(
				MetricsTypePrometheus,
				MetricsTypeNoop,
			).Error("must be one of: prometheus, noop"),
		),
		validation.Field(&m.Adapters),
	)
}

// Validate performs validation on the MetricsAdapters configuration.
func (a MetricsAdapters) Validate() error {
	return ValidateStruct(&a,
		validation.Field(&a.Prometheus),
	)
}

// MetricsPrometheus contains Prometheus-specific configuration.
type MetricsPrometheus struct {
	// CustomRegistry determines whether to use a custom Prometheus registry
	// instead of the default global registry.
	// Use this when running tests or when you need isolated metrics.
	CustomRegistry bool `yaml:"customRegistry" default:"false"`
}

// Validate performs validation on the MetricsPrometheus configuration.
func (m MetricsPrometheus) Validate() error {
	return nil
}
